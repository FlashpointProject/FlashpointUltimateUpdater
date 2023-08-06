package main

import (
	"context"
	"crypto/sha1"
	"database/sql"
	"encoding/hex"
	"fmt"
	"fyne.io/fyne/v2/dialog"
	"github.com/cavaliergopher/grab/v3"
	"path/filepath"
	"sync"
	"time"
)

type Update struct {
	IndexFile *IndexedFile
	Progress  float64
	Done      bool
}

type Downloader struct {
	RateLimit   *int
	state       *InstallerState
	ctx         context.Context
	cancel      context.CancelFunc
	client      *grab.Client
	reqch       chan *grab.Request
	respch      chan *grab.Response
	updatech    chan *Update
	workerWg    sync.WaitGroup
	responderWg sync.WaitGroup
	updaterWg   sync.WaitGroup
	running     bool
	started     bool
	installPath string
}

func NewDownloader(state *InstallerState) *Downloader {
	d := &Downloader{
		RateLimit:   nil,
		state:       state,
		client:      grab.NewClient(),
		workerWg:    sync.WaitGroup{},
		responderWg: sync.WaitGroup{},
		updaterWg:   sync.WaitGroup{},
		running:     false,
		started:     false,
	}

	return d
}

func (d *Downloader) Resume() error {
	if d.running {
		return nil
	}
	installPath, err := d.state.folderPath.Get()
	if err != nil {
		return err
	}
	d.installPath = installPath

	// Reset context
	d.ctx, d.cancel = context.WithCancel(context.Background())

	// Set up background
	d.reqch = make(chan *grab.Request, 10)
	d.respch = make(chan *grab.Response, 4)
	d.updatech = make(chan *Update, 4)

	// Add 4 workers
	for i := 0; i < 4; i++ {
		d.workerWg.Add(1)
		go func() {
			defer d.workerWg.Done()
			d.client.DoChannel(d.reqch, d.respch)
		}()
	}

	// Add a receiver for responses
	d.responderWg.Add(1)
	go func() {
		for resp := range d.respch {
			d.responderWg.Add(1)
			// Spin up goroutine for each response
			go func(resp *grab.Response) {
				defer d.responderWg.Done()
				t := time.NewTicker(500 * time.Millisecond)
				defer t.Stop()

				for {
					select {
					case <-t.C:
						// Update in-progress values here
						f := resp.Request.Tag.(*IndexedFile)
						d.updatech <- &Update{
							IndexFile: f,
							Progress:  resp.Progress(),
							Done:      false,
						}
					case <-resp.Done:
						// Done, check for error
						err := resp.Err()
						if err != nil {
							fmt.Println(err.Error())
							// Bad download
						} else {
							// Successful download, notify UI updater
							f := resp.Request.Tag.(*IndexedFile)
							d.updatech <- &Update{
								IndexFile: f,
								Progress:  1,
								Done:      true,
							}
						}
						return
					}
				}
			}(resp)
		}
		// Channel closed, finish out
		d.responderWg.Done()
	}()

	// Set up UI updater
	d.updaterWg.Add(1)
	go func() {
		defer d.updaterWg.Done()
		// Create UI files
		uifiles := make([]*UiFile, 4)
		for i := 0; i < 4; i++ {
			uifiles[i] = &UiFile{
				Filepath: "",
				Progress: 0,
				Done:     true,
			}
		}

		for update := range d.updatech {
			fmt.Println(fmt.Sprintf("path: %s, progress: %f", update.IndexFile.Filepath, update.Progress))
			// Update UI element
			updateIdx := -1
			for idx, f := range uifiles {
				if f.Filepath == update.IndexFile.Filepath {
					f.Progress = update.Progress
					f.Done = update.Done
					updateIdx = idx
					break
				}
			}
			if updateIdx == -1 {
				// Didn't find existing entry, find an older one to replace
				for idx, f := range uifiles {
					if f.Done == true {
						f.Filepath = update.IndexFile.Filepath
						f.Progress = update.Progress
						f.Done = update.Done
						updateIdx = idx
						break
					}
				}
			}
			// If updated ui state, update element
			if updateIdx != -1 {
				if updateIdx == 0 {
					_ = d.state.fileTitle1.Set(update.IndexFile.Filepath)
					_ = d.state.fileProgress1.Set(update.Progress)
				}
				if updateIdx == 1 {
					_ = d.state.fileTitle2.Set(update.IndexFile.Filepath)
					_ = d.state.fileProgress2.Set(update.Progress)
				}
				if updateIdx == 2 {
					_ = d.state.fileTitle3.Set(update.IndexFile.Filepath)
					_ = d.state.fileProgress3.Set(update.Progress)
				}
				if updateIdx == 3 {
					_ = d.state.fileTitle4.Set(update.IndexFile.Filepath)
					_ = d.state.fileProgress4.Set(update.Progress)
				}

			}

			if update.Done {
				// Mark as done
				err := d.state.Repo.MarkFileDone(update.IndexFile)
				if err != nil {
					dialog.NewError(&DatabaseError{err}, d.state.window).Show()
				}

				// Add new request if still running
				go func() {
					select {
					case <-d.ctx.Done():
						{
							// Kill request channel since we are sole sender
							close(d.reqch)
							return
						}
					default:
						{
							// Add new request to the queue
							f, err := d.state.Repo.GetNextFile()
							if err != nil {
								if err != sql.ErrNoRows {
									dialog.NewError(&DatabaseError{err}, d.state.window).Show()
								}
							} else {
								req, err := d.NewRequest(f)
								if err != nil {
									dialog.NewError(err, d.state.window).Show()
								}
								d.reqch <- req
							}
						}
					}
				}()

				// Update Total Progress bar state
				d.state.downloadedSize += update.IndexFile.Size
				d.state.downloadedFiles += 1
				err = d.state.formatDownloadedSize.Set(FormatBytes(d.state.downloadedSize))
				if err != nil {
					dialog.NewError(err, d.state.window).Show()
				}
				progress := float64(d.state.downloadedSize) / float64(d.state.totalSize)
				fmt.Println(progress)
				err = d.state.progressBarTotal.Set(progress)
				if err != nil {
					dialog.NewError(err, d.state.window).Show()
				}

			}
		}
	}()

	// Add initial 10 files
	files, err := d.state.Repo.GetNextFileBatch(10)
	if err != nil {
		return err
	}
	for _, f := range files {
		req, err := d.NewRequest(f)
		if err != nil {
			return err
		}

		// Add request to queue
		d.reqch <- req
	}
	d.running = true
	d.started = true

	return nil
}

func (d *Downloader) Stop() {
	if !d.running {
		return
	}

	// Stop new and current requests
	d.cancel()

	// Wait for all workers to exit
	d.workerWg.Wait()
	close(d.respch)

	// Wait for all responses to finish processing
	d.responderWg.Wait()
	close(d.updatech)
	d.updaterWg.Wait()

	d.running = false
}

func (d *Downloader) NewRequest(f *IndexedFile) (*grab.Request, error) {
	// Set up request
	dest := filepath.Join(d.installPath, f.Filepath)
	req, err := grab.NewRequest(dest, fmt.Sprintf("%s/%s", d.state.baseUrl, f.Filepath))
	if err != nil {
		return nil, err
	}

	// Add automatic checksumming
	sum, err := hex.DecodeString(f.SHA1)
	if err != nil {
		return nil, err
	}
	req.SetChecksum(sha1.New(), sum, true)
	req.Size = f.Size
	req = req.WithContext(d.ctx)

	// Add indexed file as tag
	req.Tag = f

	return req, nil
}
