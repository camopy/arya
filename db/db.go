package db

import (
	"context"
	"time"
)

type DB interface {
	Get(ctx context.Context, key string) ([]byte, error)
	Set(ctx context.Context, key string, value []byte, ttl time.Duration) error
	Add(ctx context.Context, key string, value []byte) (id string, err error)
	List(ctx context.Context, key string) (map[string]string, error)
	Del(ctx context.Context, key string, id string) error
	IsErrNotFound(err error) bool
	//Close() error
}
