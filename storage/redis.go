package storage

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"awning-backend/model"

	"github.com/redis/go-redis/v9"
)

// RedisClient wraps the Redis client with chat storage operations
type RedisClient struct {
	client *redis.Client
}

// NewRedisClient creates a new Redis client
func NewRedisClient(addr, password string, db int) (*RedisClient, error) {
	client := redis.NewClient(&redis.Options{
		Addr:     addr,
		Password: password,
		DB:       db,
	})

	// Test connection
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := client.Ping(ctx).Err(); err != nil {
		return nil, fmt.Errorf("failed to connect to Redis: %w", err)
	}

	slog.Info("Redis client initialized successfully", "addr", addr)
	return &RedisClient{client: client}, nil
}

// Close closes the Redis connection
func (r *RedisClient) Close() error {
	return r.client.Close()
}

// SaveChat saves a chat to Redis
func (r *RedisClient) SaveChat(ctx context.Context, chat *model.Chat) error {
	data, err := chat.ToJSON()
	if err != nil {
		return fmt.Errorf("failed to serialize chat: %w", err)
	}

	key := fmt.Sprintf("chat:%s", chat.ID)
	err = r.client.Set(ctx, key, data, 0).Err()
	if err != nil {
		return fmt.Errorf("failed to save chat to Redis: %w", err)
	}

	slog.Debug("Chat saved to Redis", "chat_id", chat.ID)
	return nil
}

// GetChat retrieves a chat from Redis
func (r *RedisClient) GetChat(ctx context.Context, chatID string) (*model.Chat, error) {
	key := fmt.Sprintf("chat:%s", chatID)
	data, err := r.client.Get(ctx, key).Bytes()
	if err == redis.Nil {
		return nil, fmt.Errorf("chat not found: %s", chatID)
	}
	if err != nil {
		return nil, fmt.Errorf("failed to get chat from Redis: %w", err)
	}

	chat, err := model.FromJSON(data)
	if err != nil {
		return nil, fmt.Errorf("failed to deserialize chat: %w", err)
	}

	slog.Debug("Chat retrieved from Redis", "chat_id", chatID)
	return chat, nil
}

// DeleteChat deletes a chat from Redis
func (r *RedisClient) DeleteChat(ctx context.Context, chatID string) error {
	key := fmt.Sprintf("chat:%s", chatID)
	err := r.client.Del(ctx, key).Err()
	if err != nil {
		return fmt.Errorf("failed to delete chat from Redis: %w", err)
	}

	slog.Debug("Chat deleted from Redis", "chat_id", chatID)
	return nil
}

// ListChats lists all chat IDs (for debugging/admin purposes)
func (r *RedisClient) ListChats(ctx context.Context) ([]string, error) {
	keys, err := r.client.Keys(ctx, "chat:*").Result()
	if err != nil {
		return nil, fmt.Errorf("failed to list chats: %w", err)
	}

	// Strip "chat:" prefix from keys
	chatIDs := make([]string, len(keys))
	for i, key := range keys {
		chatIDs[i] = key[5:] // Remove "chat:" prefix
	}

	return chatIDs, nil
}

// Get retrieves a value from Redis by key
func (r *RedisClient) Get(ctx context.Context, key string) ([]byte, error) {
	data, err := r.client.Get(ctx, key).Bytes()
	if err == redis.Nil {
		return nil, fmt.Errorf("key not found: %s", key)
	}
	if err != nil {
		return nil, fmt.Errorf("failed to get key from Redis: %w", err)
	}
	return data, nil
}

// SetWithTTL stores a value in Redis with a TTL
func (r *RedisClient) SetWithTTL(ctx context.Context, key string, value []byte, ttl time.Duration) error {
	err := r.client.Set(ctx, key, value, ttl).Err()
	if err != nil {
		return fmt.Errorf("failed to set key in Redis: %w", err)
	}
	return nil
}

// Delete removes a key from Redis
func (r *RedisClient) Delete(ctx context.Context, key string) error {
	err := r.client.Del(ctx, key).Err()
	if err != nil {
		return fmt.Errorf("failed to delete key from Redis: %w", err)
	}
	return nil
}
