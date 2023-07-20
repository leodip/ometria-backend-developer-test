package statemgr

import (
	"context"
	"time"

	"github.com/redis/go-redis/v9"
	"github.com/spf13/viper"
)

type Redis struct {
	client *redis.Client
}

func NewRedis() *Redis {

	client := redis.NewClient(&redis.Options{
		Addr:     viper.GetString("Redis.Addr"),
		Password: viper.GetString("Redis.Password"),
		DB:       viper.GetInt("DB"),
	})

	return &Redis{
		client: client,
	}
}

func (r *Redis) SetCompletedRun(ctx context.Context, timeStamp time.Time, listId string) error {
	err := r.client.Set(ctx, listId, time.Now().Format(time.RFC3339), 0).Err()
	if err != nil {
		return err
	}
	return nil
}

func (r *Redis) GetLastRunTimestamp(ctx context.Context, listId string) (string, error) {
	val, err := r.client.Get(ctx, listId).Result()
	if err == redis.Nil {
		// key does not exist
		return "", nil
	} else if err != nil {
		return "", err
	}
	return val, nil
}
