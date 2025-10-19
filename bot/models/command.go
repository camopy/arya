package models

import (
	"fmt"
	"time"
)

var ErrInvalidIntervalDuration = fmt.Errorf("interval must be at least 60 minutes")

type Commander interface {
	Action() string
	Interval() time.Duration
	ThreadId() int
	SubName() string
	Platform() string
	Url() string
}

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
