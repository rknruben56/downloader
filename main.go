// Package main starts the downloader
package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/url"
	"os"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/s3/s3manager"
	"github.com/aws/aws-sdk-go/service/sqs"
	"github.com/wader/goutubedl"
)

var (
	qURL            string
	svc             *sqs.SQS
	sess            *session.Session
	uploader        *s3manager.Uploader
	maxMsgCount     = 2
	ytPath          = "yt-dlp"
	downloadQuality = "best"
	vIDParam        = "v"
)

// SNSMessage is the format the incoming message is in
type SNSMessage struct {
	Message string `json:"Message"`
}

func main() {
	setupResources()
	chnMessages := make(chan *sqs.Message, maxMsgCount)
	go pollSqs(chnMessages)

	fmt.Println("Listening to SQS")

	for message := range chnMessages {
		handleMessage(message)
		deleteMessage(message)
	}
}

func setupResources() {
	sess := session.Must(session.NewSessionWithOptions(session.Options{
		SharedConfigState: session.SharedConfigDisable,
	}))
	svc = sqs.New(sess)
	qURL = os.Getenv("COPILOT_QUEUE_URI")
	uploader = s3manager.NewUploader(sess)
}

func pollSqs(chn chan<- *sqs.Message) {
	for {
		result, err := svc.ReceiveMessage(&sqs.ReceiveMessageInput{
			QueueUrl:            &qURL,
			MaxNumberOfMessages: aws.Int64(int64(maxMsgCount)),
			VisibilityTimeout:   aws.Int64(60),
			WaitTimeSeconds:     aws.Int64(0),
		})
		if err != nil {
			fmt.Println("error reading message", err)
		}

		for _, message := range result.Messages {
			chn <- message
		}
	}
}

func handleMessage(message *sqs.Message) {
	videoID, err := getVideoID(message)
	if err != nil {
		fmt.Println(err)
		return
	}

	b, err := downloadVideo(videoID)
	if err != nil {
		fmt.Println(err)
		return
	}

	err = uploadToS3(videoID, b)
	if err != nil {
		fmt.Println(err)
		return
	}

	fmt.Printf("Video processed: %s", videoID)
}

func getVideoID(message *sqs.Message) (string, error) {
	snsMessage := new(SNSMessage)
	err := json.Unmarshal([]byte(*message.Body), snsMessage)
	if err != nil {
		return "", err
	}

	vidURL, err := url.Parse(snsMessage.Message)
	if err != nil {
		return "", err
	}
	params, _ := url.ParseQuery(vidURL.RawQuery)
	if _, ok := params[vIDParam]; !ok {
		return "", errors.New("videoId not found")
	}

	return params.Get(vIDParam), nil
}

func downloadVideo(url string) (*bytes.Buffer, error) {
	b := &bytes.Buffer{}
	goutubedl.Path = ytPath
	result, err := goutubedl.New(context.Background(), url, goutubedl.Options{})
	if err != nil {
		return nil, err
	}

	downloadResult, err := result.Download(context.Background(), downloadQuality)
	if err != nil {
		return nil, err
	}

	_, err = io.Copy(b, downloadResult)
	if err != nil {
		return nil, err
	}

	return b, err
}

func uploadToS3(key string, b *bytes.Buffer) error {
	_, err := uploader.Upload(&s3manager.UploadInput{
		Bucket: aws.String(os.Getenv("DOWNLOADS_NAME_BUCKET_NAME")),
		Key:    aws.String(key),
		Body:   b,
	})
	if err != nil {
		return err
	}

	return nil
}

func deleteMessage(message *sqs.Message) {
	_, err := svc.DeleteMessage(&sqs.DeleteMessageInput{
		QueueUrl:      &qURL,
		ReceiptHandle: message.ReceiptHandle,
	})
	if err != nil {
		fmt.Println("delete error", err)
		return
	}

	fmt.Println("Message deleted")
}
