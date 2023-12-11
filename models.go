package main

// Input is the format the incoming message is in
type Input struct {
	Message string `json:"Message"`
}

// Output contains the video ID to emit
type Output struct {
	VideoID string `json:"videoID"`
}

// SNSTopics contains the topics this service can emit
type SNSTopics struct {
	DownloadCompleteTopic string `json:"downloadCompleteTopic"`
}
