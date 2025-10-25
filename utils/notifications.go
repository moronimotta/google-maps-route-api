package utils

import (
	"net/http"
	"strings"
	"time"
)

type Message struct {
	Content string    `json:"content"`
	Topic   string    `json:"topic,omitempty"`
	TimeNow time.Time `json:"time_now"`
}

func SendNotification(message Message) {
	message.TimeNow = time.Now()
	http.Post("https://ntfy.sh/"+message.Topic, "text/plain",
		strings.NewReader(message.Content+"\nTime: "+message.TimeNow.Format(time.RFC3339)))
}

func FormatErrorNotification(err error, context string) Message {
	return Message{
		Content: "Error occurred: " + err.Error() + " | Context: " + context,
		Topic:   "bike-byui-hack-errors",
		TimeNow: time.Now(),
	}
}

func FormatInfoNotification(info string, context string) Message {
	return Message{
		Content: "Info: " + info + " | Context: " + context,
		Topic:   "bike-byui-hack-info",
		TimeNow: time.Now(),
	}
}
