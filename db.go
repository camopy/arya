package main

import (
	"context"
	"time"
)

const ttl = 24 * 7 * time.Hour

type DB interface {
	Get(ctx context.Context, key string) ([]byte, error)
	Put(ctx context.Context, key string, value []byte) error
	IsErrNotFound(err error) bool
	//Close() error
}
