// Package main starts the downloader
package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"os"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/s3/s3manager"
	"github.com/aws/aws-sdk-go/service/sqs"
	"github.com/rknruben56/downloader/download"
	"github.com/rknruben56/downloader/transcode"
	"github.com/rknruben56/downloader/upload"
)

var (
	sqsClient             *sqs.SQS
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
	snsTopicVar           = os.Getenv("COPILOT_SNS_TOPIC_ARNS")
	queueURIVar           = os.Getenv("COPILOT_QUEUE_URI")
	bucketNameVar         = os.Getenv("AUDIO_NAME")
	serviceDiscoveryVar   = os.Getenv("COPILOT_SERVICE_DISCOVERY_ENDPOINT")
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

	// SNS Topic
	topics := new(SNSTopics)
	err := json.Unmarshal([]byte(snsTopicVar), topics)
	if err != nil {
		fmt.Println("error reading topic from variables")
	}
	downloadCompleteTopic = topics.DownloadCompleteTopic

	// Internal resources
	downloader = &download.YTDownloader{Path: "yt-dlp"}
	transcoder = &transcode.MP3Transcoder{}
	uploader = &upload.MusicUploader{
		BucketName: bucketNameVar,
		Uploader:   s3manager.NewUploader(sess),
		Sess:       sess,
	}
}

func pollSqs(chn chan<- *sqs.Message) {
	for {
		result, err := sqsClient.ReceiveMessage(&sqs.ReceiveMessageInput{
			QueueUrl:            &queueURIVar,
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

	output := &Output{
		VideoID: videoID,
		Title:   dResult.Title,
		URL:     url,
	}
	err = completeProcess(*output)
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

func completeProcess(output Output) error {
	b, err := json.Marshal(output)
	if err != nil {
		return err
	}

	r, err := http.NewRequest("POST", fmt.Sprintf("http://api.%s/complete", serviceDiscoveryVar), bytes.NewBuffer(b))
	if err != nil {
		return err
	}

	r.Header.Add("Content-Type", "application/json")
	client := &http.Client{}
	res, err := client.Do(r)
	if err != nil {
		return err
	}

	defer res.Body.Close()

	if res.StatusCode != http.StatusOK {
		return fmt.Errorf("POST call failed. Status: %d", res.StatusCode)
	}

	return nil
}

func deleteMessage(message *sqs.Message) {
	_, err := sqsClient.DeleteMessage(&sqs.DeleteMessageInput{
		QueueUrl:      &queueURIVar,
		ReceiptHandle: message.ReceiptHandle,
	})
	if err != nil {
		fmt.Println("delete error", err)
		return
	}

	fmt.Println("Message deleted")
}
