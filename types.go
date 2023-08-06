package main

import (
	"fmt"
	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/data/binding"
)

type UiFile struct {
	Filepath string
	Progress float64
	Done     bool
}

type IndexedFile struct {
	Filepath string `json:"path"`
	Size     int64  `json:"size"`
	SHA1     string `json:"sha1"`
}

type IndexOverview struct {
	Name       string `json:"name"`
	TotalSize  int64  `json:"total_size"`
	TotalFiles int64  `json:"total_files"`
	BaseUrl    string `json:"base_url"`
}

type InstallerState struct {
	Grabber              *Downloader
	Repo                 *SqliteRepo
	window               fyne.Window
	folderPath           binding.String
	installName          binding.String
	totalFiles           int64
	totalSize            int64
	downloadedSize       int64
	downloadedFiles      int64
	downloadSpeed        int64
	baseUrl              string
	formatDownloadedSize binding.String
	formatTotalSize      binding.String
	formatDownloadSpeed  binding.String
	progressBarTotal     binding.Float
	fileProgress1        binding.Float
	fileProgress2        binding.Float
	fileProgress3        binding.Float
	fileProgress4        binding.Float
	fileTitle1           binding.String
	fileTitle2           binding.String
	fileTitle3           binding.String
	fileTitle4           binding.String
}

type NoValidPathFoundError struct{}

func (e *NoValidPathFoundError) Error() string {
	return "No valid path found"
}

type BrokenResumableState struct {
	err error
}

func (e *BrokenResumableState) Error() string {
	return fmt.Sprintf("Broken resumable state found, must use a new index\n%s", e.err.Error())
}

type FatalDownloadFailure struct {
	err error
}

func (e *FatalDownloadFailure) Error() string {
	return fmt.Sprintf("Fatal download failure\n%s", e.err.Error())
}

type DatabaseError struct {
	err error
}

func (e *DatabaseError) Error() string {
	return fmt.Sprintf("Fatal database error\n%s", e.err.Error())
}
