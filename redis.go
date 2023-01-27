package main

import (
	"context"
	"time"

	"github.com/go-redis/redis/v9"
)

type Redis struct {
	client *redis.Client
}

func NewRedis(addr string) DB {
	opt, err := redis.ParseURL(addr)
	if err != nil {
		panic(err)
	}
	return &Redis{
		client: redis.NewClient(opt),
	}
}

func (r *Redis) Get(ctx context.Context, key string) ([]byte, error) {
	return r.client.Get(ctx, key).Bytes()
}

func (r *Redis) Set(ctx context.Context, key string, value []byte, ttl time.Duration) error {
	return r.client.Set(ctx, key, value, ttl).Err()
}

func (r *Redis) Add(ctx context.Context, key string, value []byte) error {
	return r.client.HSet(ctx, key, time.Now().Unix(), value).Err()
}

func (r *Redis) List(ctx context.Context, key string) (map[string]string, error) {
	return r.client.HGetAll(ctx, key).Result()
}

func (r *Redis) IsErrNotFound(err error) bool {
	return err == redis.Nil
}
