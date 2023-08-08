package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/app"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/data/binding"
	"fyne.io/fyne/v2/dialog"
	"fyne.io/fyne/v2/layout"
	"fyne.io/fyne/v2/widget"
	"github.com/cavaliergopher/grab/v3"
	"github.com/dustin/go-humanize"
	"image/color"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"sync"
	"time"
)

func main() {
	a := app.New()
	w := a.NewWindow("Flashpoint Ultimate Updater")

	state := NewInstallState(w)
	closeDb := func() {
		if state.Repo != nil {
			_ = state.Repo.Close()
		}
	}
	defer closeDb()

	w.SetContent(setupLayout(w, state))
	w.Resize(fyne.Size{Width: 700, Height: 400})

	// Show the window
	w.ShowAndRun()
}

func NewInstallState(w fyne.Window) *InstallerState {
	state := InstallerState{
		window:                 w,
		App:                    app.NewWithID("com.flashpointarchive.ultimate-updater"),
		folderPath:             binding.NewString(),
		installName:            binding.NewString(),
		totalFiles:             0,
		totalSize:              0,
		downloadedSize:         0,
		downloadedFiles:        0,
		downloadSpeed:          0,
		downloadFailures:       0,
		runningLabel:           binding.NewString(),
		formatDownloadedFiles:  binding.NewString(),
		formatDownloadedSize:   binding.NewString(),
		formatTotalFiles:       binding.NewString(),
		formatTotalSize:        binding.NewString(),
		formatDownloadSpeed:    binding.NewString(),
		formatDownloadFailures: binding.NewString(),
		progressBarTotal:       binding.NewFloat(),
		fileTitle1:             binding.NewString(),
		fileTitle2:             binding.NewString(),
		fileTitle3:             binding.NewString(),
		fileTitle4:             binding.NewString(),
		fileProgress1:          binding.NewFloat(),
		fileProgress2:          binding.NewFloat(),
		fileProgress3:          binding.NewFloat(),
		fileProgress4:          binding.NewFloat(),
		rateLimitEntry:         binding.NewString(),
		formatRateLimit:        binding.NewString(),
		resumable:              false,
		baseUrl:                "",
	}
	_ = state.folderPath.Set("Not Set")
	_ = state.installName.Set("None")
	_ = state.formatDownloadedFiles.Set("0")
	_ = state.formatDownloadedSize.Set("0.0B")
	_ = state.formatTotalFiles.Set("0")
	_ = state.formatTotalSize.Set("0.0B")
	_ = state.formatDownloadSpeed.Set("0.0B/s")
	_ = state.progressBarTotal.Set(0)
	_ = state.fileTitle1.Set("None")
	_ = state.fileTitle2.Set("None")
	_ = state.fileTitle3.Set("None")
	_ = state.fileTitle4.Set("None")
	_ = state.fileProgress1.Set(0)
	_ = state.fileProgress2.Set(0)
	_ = state.fileProgress3.Set(0)
	_ = state.fileProgress4.Set(0)
	_ = state.rateLimitEntry.Set("")
	_ = state.formatRateLimit.Set("Unlimited")
	_ = state.runningLabel.Set("Stopped")
	_ = state.formatDownloadFailures.Set("0")

	// Try and load config
	err := loadConfig(&state)
	if err != nil {
		dialog.NewError(&ConfigError{err}, w).Show()
	} else {
		err = func() error {
			// Load meta.json from remote
			response, err := http.Get(state.Config.MetaUrl)
			if err != nil {
				fmt.Println("Error:", err)
				return err
			}
			defer response.Body.Close()

			// Read the response body into a string
			bodyBytes, err := io.ReadAll(response.Body)
			if err != nil {
				return err
			}

			var meta Meta
			decoder := json.NewDecoder(bytes.NewReader(bodyBytes))
			err = decoder.Decode(&meta)
			if err != nil {
				return err
			}

			state.Meta = &meta
			return nil
		}()
		if err != nil {
			d := dialog.NewError(&MetaError{err}, w)
			d.SetOnClosed(func() {
				state.App.Quit()
			})
			d.Show()
		} else {
			// Try and load last opened folder
			lastInstallPath := state.App.Preferences().StringWithFallback("last-install-path", "")
			if lastInstallPath != "" {
				p, resumable, err := validatePath(lastInstallPath)
				if err == nil {
					loadDatabaseResume(p, resumable, &state)
				}
				// Ignore any error and pretend the path wasn't set
			}
		}
	}

	state.Grabber = NewDownloader(&state)

	return &state
}

func loadConfig(state *InstallerState) error {
	cwd, err := os.Getwd()
	if err != nil {
		return err
	}

	parseConfig := func(content io.Reader) (*Config, error) {
		// Create a new Config instance
		var config Config

		// Decode the JSON data from the file into the Config struct
		decoder := json.NewDecoder(content)
		err = decoder.Decode(&config)
		if err != nil {
			return nil, err
		}

		return &config, nil
	}

	configPath := filepath.Join(cwd, "config.json")
	_, err = os.Stat(configPath)
	if err == nil {
		// External found, use that
		file, err := os.Open(configPath)
		if err != nil {
			return err
		}
		defer file.Close()

		config, err := parseConfig(file)
		if err != nil {
			return err
		}
		state.Config = config
	} else if os.IsNotExist(err) {
		// None found, Check internal config
		reader := bytes.NewReader(resourceConfigJson.StaticContent)
		config, err := parseConfig(reader)
		if err != nil {
			return err
		}
		state.Config = config
	} else {
		return err
	}

	return nil
}

func setupLayout(w fyne.Window, state *InstallerState) *fyne.Container {
	buttonResume := widget.NewButton("Resume Install", func() {
		w.SetContent(mainLayout(w, state))
	})
	if !state.resumable {
		buttonResume.Disable()
	}

	installName, err := state.installName.Get()
	if err != nil {
		installName = "None"
	}
	newInstallText := "New Install - " + state.Meta.Current
	if installName != "None" {
		newInstallText = "Upgrade to " + state.Meta.Current
	}
	buttonNewInstall := widget.NewButton(newInstallText, func() {
		// Close existing database connection
		if state.Repo != nil {
			err := state.Repo.Close()
			if err != nil {
				dialog.NewError(&DatabaseError{err}, state.window).Show()
				return
			}
			state.Repo = nil
		}

		// Remove current install state
		folderPath, err := state.folderPath.Get()
		dbPath := filepath.Join(folderPath, "ultimate.sqlite")

		_, err = os.Stat(dbPath)
		if err != nil {
			if !os.IsNotExist(err) {
				dialog.NewError(err, state.window).Show()
				return
			}
		} else {
			err = os.Remove(dbPath)
			if err != nil {
				dialog.NewError(err, state.window).Show()
				return
			}
		}

		// Set up progress screen
		progressData := binding.NewFloat()
		_ = progressData.Set(0)

		showProgressScreen("Downloading New Index...", state.window, progressData)
		req, err := grab.NewRequest(dbPath, state.Meta.Path)
		if err != nil {
			d := dialog.NewError(&FatalDownloadFailure{err}, state.window)
			d.SetOnClosed(func() {
				state.App.Quit()
			})
			d.Show()
			return
		}

		// Download file
		client := grab.NewClient()
		res := client.Do(req)
		wg := sync.WaitGroup{}
		wg.Add(1)
		go func() {
			defer wg.Done()
			t := time.NewTicker(100 * time.Millisecond)
			defer t.Stop()
			for {
				select {
				case <-t.C:
					_ = progressData.Set(res.Progress())
				case <-res.Done:
					return
				}
			}
		}()
		wg.Wait()

		// Check for download error
		err = res.Err()
		if err != nil {
			d := dialog.NewError(&FatalDownloadFailure{err}, state.window)
			d.SetOnClosed(func() {
				state.App.Quit()
			})
			d.Show()
			return
		}

		// Load install state
		loadDatabaseResume(folderPath, true, state)
		w.SetContent(mainLayout(state.window, state))
	})
	if installName == state.Meta.Current {
		buttonNewInstall.Disable()
	}

	showFolderPicker := func() {
		onChosen := func(f fyne.ListableURI, err error) {
			if err != nil {
				dialog.NewError(err, w).Show()
				return
			}
			if f == nil {
				return
			}

			// Validate path
			p, resumable, err := validatePath(f.Path())
			if err != nil {
				dialog.NewError(err, w).Show()
				return
			}

			loadDatabaseResume(p, resumable, state)

			installName, _ := state.installName.Get()
			if installName == state.Meta.Current {
				buttonNewInstall.Disable()
			} else {
				buttonNewInstall.Enable()
			}
		}
		dialog.ShowFolderOpen(onChosen, w)
	}

	// Create path labels
	pathHeaderLabel := widget.NewLabel("Selected Install Path:")
	pathHeaderLabel.TextStyle = fyne.TextStyle{Bold: true}
	pathLabel := widget.NewLabelWithData(state.folderPath)

	// Create version labels
	versionHeaderLabel := widget.NewLabel("Current Version:")
	versionHeaderLabel.TextStyle = fyne.TextStyle{Bold: true}
	versionLabel := widget.NewLabelWithData(state.installName)
	versionContainer := container.NewVBox(
		versionHeaderLabel,
		versionLabel)

	// Create other buttons
	buttonBrowse := widget.NewButton("Browse", showFolderPicker)

	// Adjust the layout to grow the pathContainer
	browseRow := container.New(layout.NewHBoxLayout(),
		pathLabel,
		layout.NewSpacer(),
		buttonBrowse)

	innerContainer := container.NewVBox(
		pathHeaderLabel,
		browseRow,
		versionContainer)

	line := canvas.NewLine(color.Gray{Y: 0x55})
	line.StrokeWidth = 2

	layoutContainer := container.New(layout.NewVBoxLayout(),
		topBarLayout("setup"),
		innerContainer,
		line,
		buttonResume,
		buttonNewInstall,
		layout.NewSpacer())

	return layoutContainer
}

func mainLayout(w fyne.Window, state *InstallerState) *fyne.Container {
	pathHeaderLabel := widget.NewLabel("Install Path: ")
	pathHeaderLabel.TextStyle = fyne.TextStyle{Bold: true}
	pathLabel := widget.NewLabelWithData(state.folderPath)
	pathContainer := container.New(layout.NewHBoxLayout(),
		pathHeaderLabel,
		pathLabel)

	versionHeaderLabel := widget.NewLabel("Version: ")
	versionHeaderLabel.TextStyle = fyne.TextStyle{Bold: true}
	versionLabel := widget.NewLabelWithData(state.installName)
	versionContainer := container.New(layout.NewHBoxLayout(),
		versionHeaderLabel,
		versionLabel)

	// Create active file bars
	fileLabel := widget.NewLabel("File: ")
	fileLabel.TextStyle = fyne.TextStyle{Bold: true}
	fileHeader1 := widget.NewLabelWithData(state.fileTitle1)
	fileHeader1.Alignment = fyne.TextAlignLeading
	fileHeader1.Wrapping = fyne.TextTruncate
	fileContainer1 := container.NewBorder(nil, nil, fileLabel, nil, fileHeader1)
	fileProgressBar1 := widget.NewProgressBarWithData(state.fileProgress1)

	fileHeader2 := widget.NewLabelWithData(state.fileTitle2)
	fileHeader2.Alignment = fyne.TextAlignLeading
	fileHeader2.Wrapping = fyne.TextTruncate
	fileContainer2 := container.NewBorder(nil, nil, fileLabel, nil, fileHeader2)
	fileProgressBar2 := widget.NewProgressBarWithData(state.fileProgress2)

	fileHeader3 := widget.NewLabelWithData(state.fileTitle3)
	fileHeader3.Alignment = fyne.TextAlignLeading
	fileHeader3.Wrapping = fyne.TextTruncate
	fileContainer3 := container.NewBorder(nil, nil, fileLabel, nil, fileHeader3)
	fileProgressBar3 := widget.NewProgressBarWithData(state.fileProgress3)

	fileHeader4 := widget.NewLabelWithData(state.fileTitle4)
	fileHeader4.Alignment = fyne.TextAlignLeading
	fileHeader4.Wrapping = fyne.TextTruncate
	fileContainer4 := container.NewBorder(nil, nil, fileLabel, nil, fileHeader4)
	fileProgressBar4 := widget.NewProgressBarWithData(state.fileProgress4)

	progressBarTotal := widget.NewProgressBarWithData(state.progressBarTotal)
	totalLabel := canvas.NewText("Total Progress...", color.White)

	rateLimitCurrentLabel := widget.NewLabelWithData(state.formatRateLimit)
	rateLimitEntry := widget.NewEntryWithData(state.rateLimitEntry)
	rateLimitSet := widget.NewButton("Set (KB/s)", func() {
		// Turn entry into number
		entryLimit, err := state.rateLimitEntry.Get()
		if err != nil {
			dialog.NewError(err, w).Show()
			return
		}
		rateLimit, err := strconv.Atoi(entryLimit)
		if err != nil {
			dialog.NewError(&BadRateLimit{}, w).Show()
			return
		}

		if rateLimit < 200 {
			rateLimit = 0
		}

		// Save to downloader and update UI
		if rateLimit == 0 {
			err = state.formatRateLimit.Set("Unlimited")
			if err != nil {
				dialog.NewError(err, w).Show()
				return
			}
		} else {
			err = state.formatRateLimit.Set(fmt.Sprintf("%dKB/s", rateLimit))
			if err != nil {
				dialog.NewError(err, w).Show()
				return
			}
		}

		state.Grabber.RateLimit = rateLimit * 1024

		// Restart downloader
		if state.Grabber.running {
			state.Grabber.Stop(false)
			err = state.Grabber.Resume()
			if err != nil {
				dialog.NewError(&FatalDownloadFailure{err}, w).Show()
				return
			}
		}
	})

	rateLimContainer := container.NewBorder(nil, nil, nil, rateLimitSet, rateLimitEntry)

	// Create buttons
	button1 := widget.NewButton("Start", func() {
		err := state.Grabber.Resume()
		if err != nil {
			dialog.NewError(&FatalDownloadFailure{err}, w).Show()
			return
		}
	})

	button2 := widget.NewButton("Pause", func() {
		state.Grabber.Stop(false)
	})

	button3 := widget.NewButton("Repair All Files", func() {
		dialog.NewConfirm("Are you sure?", "Repairing may take a while.", func(success bool) {
			if !success {
				return
			}

			openWaitScreen("Resetting Install State...", w)
			defer func() {
				w.SetContent(mainLayout(w, state))
			}()

			// Stop downloader
			state.Grabber.Stop(false)

			// Clear Done state for all entries
			err := state.Repo.ResetDownloadState()
			if err != nil {
				dialog.NewError(&DatabaseError{err}, w).Show()
				return
			}

			// Update progress state
			_ = state.progressBarTotal.Set(0)
			state.downloadedFiles = 0
			state.downloadedSize = 0
			_ = state.formatDownloadedFiles.Set("0")
			_ = state.formatDownloadedSize.Set("0.0B")

			// Start downloader again
			err = state.Grabber.Resume()
			if err != nil {
				dialog.NewError(&FatalDownloadFailure{err}, w).Show()
				return
			}
		}, w).Show()
	})

	runningLabel := widget.NewLabelWithData(state.runningLabel)
	runningLabel.Alignment = fyne.TextAlignCenter
	runningLabel.TextStyle = fyne.TextStyle{Bold: true}

	// Create a row with buttons
	buttonsRow := container.NewBorder(nil, nil, container.NewHBox(button1, button2, button3), nil, runningLabel)

	// Create stats labels
	downloadedLabel := widget.NewLabelWithData(state.formatDownloadedSize)
	downloadedLabel.Alignment = fyne.TextAlignLeading
	downloadedLabel.TextStyle = fyne.TextStyle{Monospace: true}
	totalSizeLabel := widget.NewLabelWithData(state.formatTotalSize)
	totalSizeLabel.Alignment = fyne.TextAlignLeading
	totalSizeLabel.TextStyle = fyne.TextStyle{Monospace: true}
	downloadedContainer := container.New(layout.NewHBoxLayout(),
		downloadedLabel,
		widget.NewLabel("/"),
		totalSizeLabel)

	filesLabel := widget.NewLabelWithData(state.formatDownloadedFiles)
	filesLabel.Alignment = fyne.TextAlignLeading
	filesLabel.TextStyle = fyne.TextStyle{Monospace: true}
	totalFilesLabel := widget.NewLabelWithData(state.formatTotalFiles)
	totalFilesLabel.Alignment = fyne.TextAlignLeading
	totalFilesLabel.TextStyle = fyne.TextStyle{Monospace: true}
	filesContainer := container.New(layout.NewHBoxLayout(),
		filesLabel,
		widget.NewLabel("/"),
		totalFilesLabel)

	speedLabel := widget.NewLabelWithData(state.formatDownloadSpeed)
	speedLabel.Alignment = fyne.TextAlignLeading
	speedLabel.TextStyle = fyne.TextStyle{Monospace: true}

	failureLabel := widget.NewLabelWithData(state.formatDownloadFailures)
	failureLabel.Alignment = fyne.TextAlignLeading
	failureLabel.TextStyle = fyne.TextStyle{Monospace: true}

	statsForm := &widget.Form{
		Items: []*widget.FormItem{
			{Text: "Downloaded:", Widget: downloadedContainer},
			{Text: "Files:", Widget: filesContainer},
			{Text: "Failures:", Widget: failureLabel},
			{Text: "Average Speed:", Widget: speedLabel},
			{Text: "Download Speed Limit:", Widget: rateLimitCurrentLabel},
		},
	}

	leftMainContent := container.New(layout.NewVBoxLayout(),
		pathContainer,
		totalLabel,
		progressBarTotal,
		statsForm,
		layout.NewSpacer(),
		rateLimContainer,
		buttonsRow,
	)

	rightMainContent := container.New(layout.NewVBoxLayout(),
		versionContainer,
		fileContainer1,
		fileProgressBar1,
		fileContainer2,
		fileProgressBar2,
		fileContainer3,
		fileProgressBar3,
		fileContainer4,
		fileProgressBar4)

	mainContent := container.NewBorder(nil, nil, leftMainContent, nil, rightMainContent)

	// Combine the grid layout and sidebar label in a horizontal box
	mainLayout := container.NewBorder(topBarLayout("install"), nil, nil, nil, mainContent)

	return mainLayout
}

func topBarLayout(activeTab string) *fyne.Container {
	label1 := widget.NewLabelWithStyle("Setup", fyne.TextAlignCenter, fyne.TextStyle{Bold: activeTab == "setup"})
	label2 := widget.NewLabel(">")
	label3 := widget.NewLabelWithStyle("Install", fyne.TextAlignCenter, fyne.TextStyle{Bold: activeTab == "install"})

	line := canvas.NewLine(color.Gray{Y: 0x55})
	line.StrokeWidth = 2

	return container.New(layout.NewVBoxLayout(),
		container.New(layout.NewHBoxLayout(),
			layout.NewSpacer(),
			label1,
			layout.NewSpacer(),
			label2,
			layout.NewSpacer(),
			label3,
			layout.NewSpacer()),
		line)
}

func validatePath(p string) (string, bool, error) {
	/** Check order of:
	* Root folder (is it empty, or an FP folder)
	* Flashpoint child folder
	 */

	Resumable := func(p string) bool {
		sqlitePath := filepath.Join(p, "ultimate.sqlite")
		_, err := os.Stat(sqlitePath)
		if os.IsNotExist(err) {
			// File does not exist
			return false
		}
		if err != nil {
			panic(err)
		}
		return true
	}

	CheckPath := func(p string) (bool, bool) {
		_, err := os.Stat(p)
		if os.IsNotExist(err) {
			// Folder does not exist
			return true, false
		}

		f, err := os.Open(p)
		if err != nil {
			// Exists, but cannot open (permissions?)
			return false, false
		}
		defer f.Close()

		_, err = f.Readdirnames(1) // Read just one entry
		if err != nil {
			// Directory empty
			return true, false
		}

		// If has a resumable file, assume a valid folder without checking others
		resumable := Resumable(p)
		if resumable {
			return true, true
		}

		// Check if existing FP folder
		folders := []string{"Data", "FPSoftware", "Launcher"}
		for _, folder := range folders {
			folderPath := filepath.Join(p, folder)
			if _, err := os.Stat(folderPath); os.IsNotExist(err) {
				return false, false
			}
		}
		return true, resumable
	}

	available, resumable := CheckPath(p)
	if available {
		return p, resumable, nil
	} else {
		p = filepath.Join(p, "Flashpoint")
		available, resumable = CheckPath(p)
		if available {
			return p, resumable, nil
		} else {
			return p, false, &NoValidPathFoundError{}
		}
	}
}

func FormatBytes(size int64) string {
	units := []string{"B", "KB", "MB", "GB"}

	var unitIndex int
	floatSize := float64(size)
	for unitIndex = 0; unitIndex < len(units)-1; unitIndex++ {
		if floatSize < 1024.0 {
			break
		}
		floatSize /= 1024.0
	}

	// Format with one decimal place
	return fmt.Sprintf("%.1f%s", floatSize, units[unitIndex])
}

func loadDatabaseResume(p string, resumable bool, state *InstallerState) {
	fmt.Printf("chosen: %v\n", p)

	err := state.folderPath.Set(p)
	if err != nil {
		panic(err)
	}
	if resumable {
		// Cover with a dialog since this can take a while!
		openWaitScreen("Loading Install State...", state.window)
		defer func() {
			state.window.SetContent(setupLayout(state.window, state))
		}()
		repo, err := OpenDatabase(filepath.Join(p, "ultimate.sqlite"))
		if err != nil {
			dialog.NewError(&BrokenResumableState{err}, state.window).Show()
			return
		}
		overview, err := repo.GetOverview()
		if err != nil {
			dialog.NewError(&BrokenResumableState{err}, state.window).Show()
			return
		}
		totalDownloadedSize, err := repo.GetTotalDownloadedSize()
		if err != nil {
			dialog.NewError(&BrokenResumableState{err}, state.window).Show()
			return
		}
		totalDownloadedFiles, err := repo.GetTotalDownladedFiles()
		if err != nil {
			dialog.NewError(&BrokenResumableState{err}, state.window).Show()
			return
		}
		_ = state.installName.Set(overview.Name)
		state.totalFiles = overview.TotalFiles
		state.totalSize = overview.TotalSize
		state.downloadedFiles = totalDownloadedFiles
		state.downloadedSize = totalDownloadedSize
		state.baseUrl = overview.BaseUrl
		_ = state.formatDownloadedFiles.Set(humanize.Comma(state.downloadedFiles))
		_ = state.formatDownloadedSize.Set(FormatBytes(state.downloadedSize))
		_ = state.formatTotalFiles.Set(humanize.Comma(state.totalFiles))
		_ = state.formatTotalSize.Set(FormatBytes(state.totalSize))
		progress := float64(state.downloadedSize) / float64(state.totalSize) * 100
		err = state.progressBarTotal.Set(progress)
		if err != nil {
			dialog.NewError(err, state.window).Show()
		}
		state.Repo = repo
		state.resumable = false
		for _, v := range state.Meta.Available {
			if v == overview.Name {
				state.resumable = true
			}
		}
		if !state.resumable {
			dialog.NewError(&VersionTooOld{}, state.window).Show()
		}
		state.App.Preferences().SetString("last-install-path", p)
	} else {
		state.resumable = false
		_ = state.installName.Set("None")
		if state.Repo != nil {
			err := state.Repo.Close()
			if err != nil {
				dialog.NewError(&DatabaseError{err}, state.window).Show()
			}
			state.Repo = nil
		}
		// Refresh setup screen
		state.window.SetContent(setupLayout(state.window, state))
	}
}

func openWaitScreen(message string, w fyne.Window) {
	// Create a dialog to show the operation status
	progressBar := widget.NewProgressBarInfinite()
	dialogContent := container.NewCenter(
		container.NewVBox(
			widget.NewLabel(message),
			progressBar),
	)

	w.SetContent(dialogContent)
}

func showProgressScreen(message string, w fyne.Window, progressData binding.Float) {
	// Create a dialog to show the operation status
	progressBar := widget.NewProgressBarWithData(progressData)
	dialogContent := container.NewCenter(
		container.NewVBox(
			widget.NewLabel(message),
			progressBar),
	)

	w.SetContent(dialogContent)
}
