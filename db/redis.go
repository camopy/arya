package db

import (
	"context"
	"fmt"
	"time"

	"github.com/go-redis/redis/v9"
)

type Redis struct {
	client *redis.Client
}

func NewRedis(uri string) DB {
	options, err := redis.ParseURL(uri)
	if err != nil {
		panic(err)
	}
	client := redis.NewClient(options)

	res := client.Ping(context.Background())
	if res.Err() != nil {
		panic(fmt.Errorf("failed to ping redis: %w", res.Err()))
	}
	return &Redis{
		client: client,
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

func (r *Redis) Del(ctx context.Context, key string, id string) error {
	return r.client.HDel(ctx, key, id).Err()
}

func (r *Redis) IsErrNotFound(err error) bool {
	return err == redis.Nil
}
