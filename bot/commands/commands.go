package commands

import "fmt"

var ErrInvalidIntervalDuration = fmt.Errorf("rss: interval must be at least 60 minutes")

type Command struct {
	Name     string
	ChatId   int64
	ThreadId int
	Text     string
}

type Content struct {
	Text     string
	ThreadId int
}
