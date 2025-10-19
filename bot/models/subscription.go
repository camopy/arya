package models

import (
	"context"
	"time"
)

type Subscription struct {
	Id       string        `json:"id"`
	Name     string        `json:"name"`
	Interval time.Duration `json:"interval"`
	ThreadId int           `json:"thread_id"`
	Platform string        `json:"platform"`
	Url      string        `json:"url"`

	CancelFunc context.CancelFunc
}
