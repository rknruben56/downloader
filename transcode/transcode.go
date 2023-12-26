package transcode

import (
	"bytes"

	fluentffmpeg "github.com/modfy/fluent-ffmpeg"
)

const format = "mp3"
const ffmpegPath = "" // empty for default

// MP3Transcoder converts the video to MP3
type MP3Transcoder struct {
}

// Transcode takes in the video as a bytes buffer and converts it to a
// buffer with MP3 bytes
func (t *MP3Transcoder) Transcode(buffer *bytes.Buffer) (*bytes.Buffer, error) {
	b := &bytes.Buffer{}
	err := fluentffmpeg.
		NewCommand(ffmpegPath).
		PipeInput(buffer).
		OutputFormat("mp3").
		PipeOutput(b).
		Run()
	return b, err
}
