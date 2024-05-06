package feeds

import (
	"context"
	"fmt"

	"github.com/prometheus/client_golang/prometheus"
	gogpt "github.com/sashabaranov/go-gpt3"
	"go.uber.org/zap"

	"github.com/camopy/rss_everything/bot/commands"
	"github.com/camopy/rss_everything/metrics"
	"github.com/camopy/rss_everything/util/psub"
	"github.com/camopy/rss_everything/util/run"
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
	defaultPrompt string

	contentPublisher psub.Publisher[[]commands.Content]
}

func NewChatGPT(logger *zaplog.Logger, contentPublisher psub.Publisher[[]commands.Content], apiKey string, userName string) *ChatGPT {
	return &ChatGPT{
		Client:        gogpt.NewClient(apiKey),
		logger:        logger,
		userName:      userName,
		defaultPrompt: fmt.Sprintf(defaultPrompt, userName, userName, "%s"),

		contentPublisher: contentPublisher,
	}
}

func (c *ChatGPT) Name() string {
	return "chat-gpt-service"
}

func (c *ChatGPT) Start(ctx run.Context) error {
	c.logger.Info("starting chat gpt")
	return nil
}

func (c *ChatGPT) ProcessPrompt(ctx context.Context, prompt commands.Content) {
	resp, err := c.processPrompt(prompt.Text)
	if err != nil {
		c.logger.Error("failed to process prompt", zap.Error(err))
		return
	}

	c.logger.Info("sending answer", zap.Int("threadId", prompt.ThreadId))
	_ = c.contentPublisher.SendData(ctx, []commands.Content{
		{
			Text:     resp,
			ThreadId: prompt.ThreadId,
		},
	})
}

func (c *ChatGPT) processPrompt(prompt string) (string, error) {
	c.logger.Info("sending prompt", zap.String("prompt", prompt))
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

func trackCompletionRequestDuration() (stop func()) {
	return metrics.TrackDuration(chatGPTMetrics.completionRequestsDuration.WithLabelValues().Observe)
}
