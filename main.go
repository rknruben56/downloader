// Package main starts the downloader
package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"os"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/s3/s3manager"
	"github.com/aws/aws-sdk-go/service/sns"
	"github.com/aws/aws-sdk-go/service/sqs"
	"github.com/rknruben56/downloader/download"
	"github.com/rknruben56/downloader/transcode"
	"github.com/rknruben56/downloader/upload"
)

var (
	qURL                  string
	sqsClient             *sqs.SQS
	snsClient             *sns.SNS
	sess                  *session.Session
	maxMsgCount           = 2
	ytPath                = "yt-dlp"
	downloadQuality       = "best"
	vIDParam              = "v"
	downloadCompleteTopic string
	pollWaitTime          = 20
	downloader            download.Downloader
	transcoder            transcode.Transcoder
	uploader              upload.Uploader
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
	snsClient = sns.New(sess)

	// SNS Topic
	topics := new(SNSTopics)
	err := json.Unmarshal([]byte(os.Getenv("COPILOT_SNS_TOPIC_ARNS")), topics)
	if err != nil {
		fmt.Println("error reading topic from variables")
	}
	downloadCompleteTopic = topics.DownloadCompleteTopic

	qURL = os.Getenv("COPILOT_QUEUE_URI")

	// Internal resources
	downloader = &download.YTDownloader{Path: "yt-dlp"}
	transcoder = &transcode.MP3Transcoder{}
	uploader = &upload.MusicUploader{
		BucketName: os.Getenv("AUDIO_NAME"),
		Uploader:   s3manager.NewUploader(sess),
		Sess:       sess,
	}
}

func pollSqs(chn chan<- *sqs.Message) {
	for {
		result, err := sqsClient.ReceiveMessage(&sqs.ReceiveMessageInput{
			QueueUrl:            &qURL,
			MaxNumberOfMessages: aws.Int64(int64(maxMsgCount)),
			VisibilityTimeout:   aws.Int64(60),
			WaitTimeSeconds:     aws.Int64(int64(pollWaitTime)),
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

	dResult, err := downloader.Download(videoID)
	if err != nil {
		fmt.Println(err)
		return
	}

	tBuff, err := transcoder.Transcode(dResult.Content)
	if err != nil {
		fmt.Println(err)
		return
	}

	url, err := uploader.Upload(videoID, tBuff)
	if err != nil {
		fmt.Println(err)
		return
	}

	err = emitDownloadComplete(videoID, dResult.Title, url)
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

func emitDownloadComplete(videoID string, title string, url string) error {
	output := Output{
		VideoID: videoID,
		Title:   title,
		URL:     url,
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
