// Package download handles the logic for downloading videos
package download

// Downloader handles the video interaction
type Downloader interface {
	Download(videoID string) (Result, error)
}
