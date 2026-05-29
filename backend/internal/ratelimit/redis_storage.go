package ratelimit

import (
	"context"
	"time"

	"github.com/redis/go-redis/v9"
)

type RedisStorage struct {
	client *redis.Client
	prefix string
}

func NewRedisStorage(client *redis.Client, prefix string) *RedisStorage {
	return &RedisStorage{client: client, prefix: prefix}
}

func (s *RedisStorage) GetWithContext(ctx context.Context, key string) ([]byte, error) {
	value, err := s.client.Get(ctx, s.key(key)).Bytes()
	if err == redis.Nil {
		return nil, nil
	}
	return value, err
}

func (s *RedisStorage) Get(key string) ([]byte, error) {
	return s.GetWithContext(context.Background(), key)
}

func (s *RedisStorage) SetWithContext(ctx context.Context, key string, val []byte, exp time.Duration) error {
	if key == "" || len(val) == 0 {
		return nil
	}
	return s.client.Set(ctx, s.key(key), val, exp).Err()
}

func (s *RedisStorage) Set(key string, val []byte, exp time.Duration) error {
	return s.SetWithContext(context.Background(), key, val, exp)
}

func (s *RedisStorage) DeleteWithContext(ctx context.Context, key string) error {
	return s.client.Del(ctx, s.key(key)).Err()
}

func (s *RedisStorage) Delete(key string) error {
	return s.DeleteWithContext(context.Background(), key)
}

func (s *RedisStorage) ResetWithContext(ctx context.Context) error {
	iter := s.client.Scan(ctx, 0, s.prefix+"*", 100).Iterator()
	for iter.Next(ctx) {
		if err := s.client.Del(ctx, iter.Val()).Err(); err != nil {
			return err
		}
	}
	return iter.Err()
}

func (s *RedisStorage) Reset() error {
	return s.ResetWithContext(context.Background())
}

func (s *RedisStorage) Close() error {
	return nil
}

func (s *RedisStorage) key(key string) string {
	return s.prefix + key
}
