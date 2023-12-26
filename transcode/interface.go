// Package transcode handles the video -> audio conversion
package transcode

import "bytes"

// Transcoder converts the video to MP3
type Transcoder interface {
	Transcode(buffer *bytes.Buffer) (*bytes.Buffer, error)
}
