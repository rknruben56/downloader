package upload

import (
	"bytes"
	"time"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/s3"
	"github.com/aws/aws-sdk-go/service/s3/s3manager"
)

// MusicUploader uploads music to S3
type MusicUploader struct {
	BucketName string
	Uploader   *s3manager.Uploader
	Sess       *session.Session
}

// Upload uploads the MP3 bytes to an S3 bucket and returns a pre-signed URL
func (u *MusicUploader) Upload(key string, b *bytes.Buffer) (string, error) {
	_, err := u.Uploader.Upload(&s3manager.UploadInput{
		Bucket:      aws.String(u.BucketName),
		Key:         aws.String(key),
		Body:        b,
		ContentType: aws.String("audio/mpeg"),
	})
	if err != nil {
		return "", err
	}

	svc := s3.New(u.Sess)
	req, _ := svc.GetObjectRequest(&s3.GetObjectInput{
		Bucket: &u.BucketName,
		Key:    &key,
	})
	urlStr, err := req.Presign(15 * time.Minute)
	if err != nil {
		return "", err
	}

	return urlStr, nil
}
