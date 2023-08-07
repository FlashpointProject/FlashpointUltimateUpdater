package main

import (
	"context"
	"fmt"
	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/data/binding"
	"time"
)

type UiFile struct {
	Filepath string
	Progress float64
	Bytes    int64
	Done     bool
}

type IndexedFile struct {
	Filepath   string `json:"path"`
	Size       int64  `json:"size"`
	CRC32      int    `json:"crc32"`
	RetryCount int
}

type IndexOverview struct {
	Name       string `json:"name"`
	TotalSize  int64  `json:"total_size"`
	TotalFiles int64  `json:"total_files"`
	BaseUrl    string `json:"base_url"`
}

type InstallerState struct {
	Busy                 bool // Prevent button presses colliding mid-execution
	Grabber              *Downloader
	Repo                 *SqliteRepo
	window               fyne.Window
	folderPath           binding.String
	installName          binding.String
	totalFiles           int64
	totalSize            int64
	downloadedSize       int64
	downloadedFiles      int64
	downloadSpeed        float64
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
	rateLimitEntry       binding.String
	formatRateLimit      binding.String
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

type BadRateLimit struct{}

func (e *BadRateLimit) Error() string {
	return "Invalid rate limit"
}

type DownloadRateLimiter struct {
	rate  int
	total int64
}

func (c *DownloadRateLimiter) WaitN(_ context.Context, bytes int) (err error) {
	c.total += int64(bytes)
	time.Sleep(
		time.Duration(1.00 / float64(c.rate) * float64(bytes) * float64(time.Second)))
	return
}
