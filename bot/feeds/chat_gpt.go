package feeds

import (
	"context"
	"fmt"

	"github.com/prometheus/client_golang/prometheus"
	gogpt "github.com/sashabaranov/go-gpt3"
	"go.uber.org/zap"

	"github.com/camopy/rss_everything/bot/commands"
	"github.com/camopy/rss_everything/metrics"
	"github.com/camopy/rss_everything/zaplog"
)

var defaultPrompt = "The following is a conversation with an AI assistant. The assistant is helpful, creative, clever, and very friendly.\n\n%s: Hello, who are you?\nAI: I am an AI created by OpenAI. How can I help you today?\n\n%s: %s\nAI:"

var chatGPTMetrics = struct {
	completionRequestsDuration *prometheus.HistogramVec
}{
	completionRequestsDuration: metrics.NewHistogramVec(
		metrics.Subsystem,
		"chatgpt_completion_request_duration_seconds",
		"Duration of completion request in seconds",
		[]string{},
		prometheus.DefBuckets,
	),
}

type ChatGPT struct {
	*gogpt.Client
	logger        *zaplog.Logger
	userName      string
	contentCh     chan []commands.Content
	promptCh      chan commands.Content
	defaultPrompt string
}

func NewChatGPT(logger *zaplog.Logger, contentCh chan []commands.Content, apiKey string, userName string) *ChatGPT {
	return &ChatGPT{
		Client:        gogpt.NewClient(apiKey),
		logger:        logger,
		userName:      userName,
		contentCh:     contentCh,
		promptCh:      make(chan commands.Content),
		defaultPrompt: fmt.Sprintf(defaultPrompt, userName, userName, "%s"),
	}
}

func (c *ChatGPT) StartChatGPT() {
	for {
		select {
		case prompt := <-c.promptCh:
			resp, err := c.ask(prompt.Text)
			if err != nil {
				c.logger.Error("failed to ask", zap.Error(err))
			}
			c.logger.Info("sending answer", zap.Int("threadId", prompt.ThreadId))
			c.contentCh <- []commands.Content{
				{
					Text:     resp,
					ThreadId: prompt.ThreadId,
				},
			}
		}
	}
}

func (c *ChatGPT) ask(prompt string) (string, error) {
	defer trackCompletionRequestDuration()()
	req := gogpt.CompletionRequest{
		Model:     gogpt.GPT3TextDavinci003,
		Prompt:    fmt.Sprintf(c.defaultPrompt, prompt),
		MaxTokens: 200,
		Stop:      []string{"AI:", fmt.Sprintf("%s:", c.userName)},
		User:      c.userName,
	}
	resp, err := c.CreateCompletion(context.Background(), req)
	if err != nil {
		return "", err
	}
	return resp.Choices[0].Text, nil
}

func (c *ChatGPT) Ask(prompt commands.Content) {
	c.promptCh <- prompt
}

func trackCompletionRequestDuration() (stop func()) {
	return metrics.TrackDuration(chatGPTMetrics.completionRequestsDuration.WithLabelValues().Observe)
}
