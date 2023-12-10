// Package main starts the downloader
package main

import (
	"fmt"
	"os"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/sqs"
)

var (
	qURL        string
	svc         *sqs.SQS
	sess        *session.Session
	maxMsgCount = 2
)

func main() {
	sess := session.Must(session.NewSessionWithOptions(session.Options{
		SharedConfigState: session.SharedConfigDisable,
	}))
	svc = sqs.New(sess)
	qURL = os.Getenv("COPILOT_QUEUE_URI")

	chnMessages := make(chan *sqs.Message, maxMsgCount)
	go pollSqs(chnMessages)

	fmt.Println("Listening to SQS")

	for message := range chnMessages {
		handleMessage(message)
		deleteMessage(message)
	}
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
	// TODO: IMPLEMENT
}

func deleteMessage(message *sqs.Message) {
	result, err := svc.DeleteMessage(&sqs.DeleteMessageInput{
		QueueUrl:      &qURL,
		ReceiptHandle: message.ReceiptHandle,
	})
	if err != nil {
		fmt.Println("delete error", err)
	}

	fmt.Println("Message deleted", result)
}
