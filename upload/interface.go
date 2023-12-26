// Package upload handles the storage interaction
package upload

import "bytes"

// Uploader uploads the given object to the specified key and returns a pre-signed URL
type Uploader interface {
	Upload(key string, b *bytes.Buffer) (string, error)
}
