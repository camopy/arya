package main

import (
	"context"
	"fmt"
	"github.com/prometheus/client_golang/prometheus"
	gogpt "github.com/sashabaranov/go-gpt3"
)

var defaultPrompt = "The following is a conversation with an AI assistant. The assistant is helpful, creative, clever, and very friendly.\n\nPaulo Camopy: Hello, who are you?\nAI: I am an AI created by OpenAI. How can I help you today?\n\nPaulo Camopy: %s\nAI:"

var chatGPTMetrics = struct {
	completionRequestsDuration *prometheus.HistogramVec
}{
	completionRequestsDuration: NewHistogramVec(
		subsystem,
		"chatgpt_completion_request_duration_seconds",
		"Duration of completion request in seconds",
		[]string{},
		prometheus.DefBuckets,
	),
}

type ChatGPT struct {
	*gogpt.Client
	contentCh chan []string
	promptCh  chan string
}

func NewChatGPT(contentCh chan []string, apiKey string) *ChatGPT {
	return &ChatGPT{
		Client:    gogpt.NewClient(apiKey),
		contentCh: contentCh,
		promptCh:  make(chan string),
	}
}

func (c *ChatGPT) StartChatGPT() {
	for {
		select {
		case prompt := <-c.promptCh:
			resp, err := c.ask(prompt)
			if err != nil {
				fmt.Println(err)
			}
			c.contentCh <- []string{resp}
		}
	}
}

func (c *ChatGPT) ask(prompt string) (string, error) {
	defer trackCompletionRequestDuration()()
	req := gogpt.CompletionRequest{
		Model:     gogpt.GPT3TextDavinci003,
		Prompt:    fmt.Sprintf(defaultPrompt, prompt),
		MaxTokens: 200,
		//Temperature: 0,
		Stop: []string{"AI:", "Paulo Camopy:"},
		User: "Paulo Camopy",
	}
	resp, err := c.CreateCompletion(context.Background(), req)
	if err != nil {
		return "", err
	}
	return resp.Choices[0].Text, nil
}

func (c *ChatGPT) Ask(prompt string) {
	c.promptCh <- prompt
}

func trackCompletionRequestDuration() (stop func()) {
	return trackDuration(chatGPTMetrics.completionRequestsDuration.WithLabelValues().Observe)
}
