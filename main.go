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
	"github.com/aws/aws-sdk-go/service/sns"
	"github.com/aws/aws-sdk-go/service/sqs"
	"github.com/wader/goutubedl"
)

var (
	qURL                  string
	sqsClient             *sqs.SQS
	snsClient             *sns.SNS
	sess                  *session.Session
	uploader              *s3manager.Uploader
	maxMsgCount           = 2
	ytPath                = "yt-dlp"
	downloadQuality       = "best"
	vIDParam              = "v"
	downloadCompleteTopic string
)

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
	// AWS Resources
	sess := session.Must(session.NewSessionWithOptions(session.Options{
		SharedConfigState: session.SharedConfigDisable,
	}))
	sqsClient = sqs.New(sess)
	uploader = s3manager.NewUploader(sess)
	snsClient = sns.New(sess)

	// SNS Topic
	topics := new(SNSTopics)
	err := json.Unmarshal([]byte(os.Getenv("COPILOT_SNS_TOPIC_ARNS")), topics)
	if err != nil {
		fmt.Println("error reading topic from variables")
	}
	downloadCompleteTopic = topics.DownloadCompleteTopic

	qURL = os.Getenv("COPILOT_QUEUE_URI")
}

func pollSqs(chn chan<- *sqs.Message) {
	for {
		result, err := sqsClient.ReceiveMessage(&sqs.ReceiveMessageInput{
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

	err = emitDownloadComplete(videoID)
	if err != nil {
		fmt.Println(err)
		return
	}

	fmt.Printf("Video processed: %s\n", videoID)
}

func getVideoID(message *sqs.Message) (string, error) {
	snsMessage := new(Input)
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

func emitDownloadComplete(videoID string) error {
	output := Output{
		VideoID: videoID,
	}

	message, _ := json.Marshal(output)
	req, _ := snsClient.PublishRequest(&sns.PublishInput{
		TopicArn: aws.String(downloadCompleteTopic),
		Message:  aws.String(string(message)),
	})

	return req.Send()
}

func deleteMessage(message *sqs.Message) {
	_, err := sqsClient.DeleteMessage(&sqs.DeleteMessageInput{
		QueueUrl:      &qURL,
		ReceiptHandle: message.ReceiptHandle,
	})
	if err != nil {
		fmt.Println("delete error", err)
		return
	}

	fmt.Println("Message deleted")
}
