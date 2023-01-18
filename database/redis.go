package database

import (
	"context"

	"github.com/go-redis/redis/v9"
)

type Redis struct {
	client *redis.Client
}

func NewRedis(addr string) DB {
	return &Redis{
		client: redis.NewClient(&redis.Options{
			Addr: addr,
			DB:   0,
		}),
	}
}

func (r *Redis) Get(ctx context.Context, key string) ([]byte, error) {
	return r.client.Get(ctx, key).Bytes()
}

func (r *Redis) Put(ctx context.Context, key string, value []byte) error {
	return r.client.Set(ctx, key, value, ttl).Err()
}

func (r *Redis) IsErrNotFound(err error) bool {
	return err == redis.Nil
}
