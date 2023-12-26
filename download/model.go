package download

import "bytes"

// Result is the object returned by the Download function
type Result struct {
	Title   string
	Content *bytes.Buffer
}
