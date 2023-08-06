package main

import (
	"fmt"
	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/app"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/data/binding"
	"fyne.io/fyne/v2/dialog"
	"fyne.io/fyne/v2/layout"
	"fyne.io/fyne/v2/widget"
	"image/color"
	"os"
	"path/filepath"
)

func main() {
	a := app.New()
	w := a.NewWindow("Flashpoint Ultimate Updater")

	state := NewInstallState(w)
	closeDb := func() {
		_ = state.Repo.db.Close()
	}
	defer closeDb()

	w.SetContent(setupLayout(w, state))
	w.Resize(fyne.Size{Width: 500, Height: 400})

	// Show the window
	w.ShowAndRun()
}

func NewInstallState(w fyne.Window) *InstallerState {
	state := InstallerState{
		window:               w,
		folderPath:           binding.NewString(),
		installName:          binding.NewString(),
		totalFiles:           0,
		totalSize:            0,
		downloadedSize:       0,
		downloadedFiles:      0,
		downloadSpeed:        0,
		formatDownloadedSize: binding.NewString(),
		formatTotalSize:      binding.NewString(),
		formatDownloadSpeed:  binding.NewString(),
		progressBarTotal:     binding.NewFloat(),
		fileTitle1:           binding.NewString(),
		fileTitle2:           binding.NewString(),
		fileTitle3:           binding.NewString(),
		fileTitle4:           binding.NewString(),
		fileProgress1:        binding.NewFloat(),
		fileProgress2:        binding.NewFloat(),
		fileProgress3:        binding.NewFloat(),
		fileProgress4:        binding.NewFloat(),
		baseUrl:              "",
	}
	_ = state.folderPath.Set("Not Set")
	_ = state.installName.Set("None")
	_ = state.formatDownloadedSize.Set("0.0B")
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

	state.Grabber = NewDownloader(&state)
	return &state
}

func setupLayout(w fyne.Window, state *InstallerState) *fyne.Container {
	buttonResume := widget.NewButton("Resume Install", func() {
		w.SetContent(mainLayout(w, state))
	})
	buttonResume.Disable()

	showFolderPicker := func() {
		onChosen := func(f fyne.ListableURI, err error) {
			if err != nil {
				panic(err)
			}
			if f == nil {
				return
			}
			fmt.Printf("chosen: %v\n", f.Path())
			// Validate path
			p, resumable, err := validatePath(f.Path())
			if err != nil {
				dialog.NewError(err, w).Show()
				return
			}
			err = state.folderPath.Set(p)
			if err != nil {
				panic(err)
			}
			if resumable {
				repo, err := OpenDatabase(filepath.Join(p, "ultimate.sqlite"))
				if err != nil {
					dialog.NewError(&BrokenResumableState{err}, w).Show()
					return
				}
				overview, err := repo.GetOverview()
				if err != nil {
					dialog.NewError(&BrokenResumableState{err}, w).Show()
					return
				}
				totalDownloadedSize, err := repo.GetTotalDownloadedSize()
				if err != nil {
					dialog.NewError(&BrokenResumableState{err}, w).Show()
					return
				}
				_ = state.installName.Set(overview.Name)
				state.totalFiles = overview.TotalFiles
				state.totalSize = overview.TotalSize
				state.downloadedSize = totalDownloadedSize
				state.baseUrl = overview.BaseUrl
				_ = state.formatDownloadedSize.Set(FormatBytes(state.downloadedSize))
				_ = state.formatTotalSize.Set(FormatBytes(state.totalSize))
				progress := float64(state.downloadedSize) / float64(state.totalSize) * 100
				err = state.progressBarTotal.Set(progress)
				if err != nil {
					dialog.NewError(err, w).Show()
				}
				state.Repo = repo
				buttonResume.Enable()
			}
		}
		dialog.ShowFolderOpen(onChosen, w)
	}

	// Create path labels
	pathHeaderLabel := widget.NewLabel("Selected Install Path:")
	pathLabel := widget.NewLabelWithData(state.folderPath)

	// Create other buttons
	buttonBrowse := widget.NewButton("Browse", showFolderPicker)

	pathContainer := container.New(layout.NewVBoxLayout(),
		pathHeaderLabel,
		pathLabel)

	// Adjust the layout to grow the pathContainer
	innerContainer := container.New(layout.NewHBoxLayout(),
		pathContainer,
		layout.NewSpacer(),
		buttonBrowse)

	line := canvas.NewLine(color.Gray{Y: 0x55})
	line.StrokeWidth = 2

	layoutContainer := container.New(layout.NewVBoxLayout(),
		topBarLayout("setup"),
		innerContainer,
		line,
		buttonResume,
		layout.NewSpacer())

	return layoutContainer
}

func mainLayout(w fyne.Window, state *InstallerState) *fyne.Container {
	pathHeaderLabel := widget.NewLabel("Install Path: ")
	pathLabel := widget.NewLabelWithData(state.folderPath)
	pathContainer := container.New(layout.NewHBoxLayout(),
		pathHeaderLabel,
		pathLabel)

	sourceHeaderLabel := widget.NewLabel("Source: ")
	sourceLabel := widget.NewLabel(state.baseUrl)
	sourceContainer := container.New(layout.NewHBoxLayout(),
		sourceHeaderLabel,
		sourceLabel)

	// Create active file bars
	fileLabel := widget.NewLabel("File: ")
	fileHeader1 := widget.NewLabelWithData(state.fileTitle1)
	fileContainer1 := container.New(layout.NewHBoxLayout(),
		fileLabel,
		fileHeader1)
	fileProgressBar1 := widget.NewProgressBarWithData(state.fileProgress1)

	fileHeader2 := widget.NewLabelWithData(state.fileTitle2)
	fileContainer2 := container.New(layout.NewHBoxLayout(),
		fileLabel,
		fileHeader2)
	fileProgressBar2 := widget.NewProgressBarWithData(state.fileProgress2)

	fileHeader3 := widget.NewLabelWithData(state.fileTitle3)
	fileContainer3 := container.New(layout.NewHBoxLayout(),
		fileLabel,
		fileHeader3)
	fileProgressBar3 := widget.NewProgressBarWithData(state.fileProgress3)

	fileHeader4 := widget.NewLabelWithData(state.fileTitle4)
	fileContainer4 := container.New(layout.NewHBoxLayout(),
		fileLabel,
		fileHeader4)
	fileProgressBar4 := widget.NewProgressBarWithData(state.fileProgress4)

	progressBarTotal := widget.NewProgressBarWithData(state.progressBarTotal)
	totalLabel := canvas.NewText("Total Progress...", color.White)

	//rateLimitLabel := widget.NewLabel("Download Speed Limit:")
	//rateLimitEntry := widget.NewEntry()

	// Create buttons
	button1 := widget.NewButton("Start", func() {
		err := state.Grabber.Resume()
		if err != nil {
			dialog.NewError(&FatalDownloadFailure{err}, w).Show()
			return
		}
	})

	button2 := widget.NewButton("Pause", func() {
		state.Grabber.Stop()
	})

	button3 := widget.NewButton("Repair All Files", func() {
		// Stop downloader
		state.Grabber.Stop()

		// Clear Done state for all entries
		err := state.Repo.ResetDownloadState()
		if err != nil {
			dialog.NewError(&DatabaseError{err}, w).Show()
			return
		}

		// Update progress state
		_ = state.progressBarTotal.Set(0)
		state.downloadedSize = 0
		_ = state.formatDownloadedSize.Set("0.0B")

		// Start downloader again
		err = state.Grabber.Resume()
		if err != nil {
			dialog.NewError(&FatalDownloadFailure{err}, w).Show()
			return
		}
	})

	// Create a row with buttons
	buttonsRow := container.NewHBox(button1, button2, button3)

	// Create a sidebar label
	sidebarLabel := widget.NewLabel("Ultimate Updater")

	// Create 3 additional labels
	downloadedHeaderLabel := widget.NewLabel("Downloaded:")
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

	speedHeaderLabel := widget.NewLabel("Average Speed:")
	speedHeaderLabel.Alignment = fyne.TextAlignLeading
	speedLabel := widget.NewLabelWithData(state.formatDownloadSpeed)
	speedLabel.Alignment = fyne.TextAlignLeading
	speedLabel.TextStyle = fyne.TextStyle{Monospace: true}

	leftMainContent := container.New(layout.NewVBoxLayout(),
		pathContainer,
		sourceContainer,
		totalLabel,
		progressBarTotal,
		layout.NewSpacer(),
		buttonsRow,
	)

	rightMainContent := container.New(layout.NewVBoxLayout(),
		fileContainer1,
		fileProgressBar1,
		fileContainer2,
		fileProgressBar2,
		fileContainer3,
		fileProgressBar3,
		fileContainer4,
		fileProgressBar4)

	mainContent := container.New(layout.NewHBoxLayout(),
		leftMainContent,
		rightMainContent)

	leftSideContent := container.New(layout.NewVBoxLayout(),
		sidebarLabel,
		layout.NewSpacer(),
		downloadedHeaderLabel,
		downloadedContainer,
		speedHeaderLabel,
		speedLabel)

	line := canvas.NewLine(color.Gray{Y: 0x55})
	line.StrokeWidth = 2

	// Combine the grid layout and sidebar label in a horizontal box
	mainLayout := container.New(layout.NewVBoxLayout(),
		topBarLayout("install"),
		container.New(layout.NewHBoxLayout(),
			leftSideContent,
			line,
			mainContent),
	)

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
	for unitIndex = 0; unitIndex < len(units); unitIndex++ {
		if floatSize < 1024.0 {
			break
		}
		floatSize /= 1024.0
	}

	// Format with one decimal place
	return fmt.Sprintf("%.1f%s", floatSize, units[unitIndex])
}
