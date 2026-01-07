package cache

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/redis/go-redis/v9"

	"github.com/bimakw/dex-aggregator/internal/domain/entities"
)

// Cache defines the interface for caching operations
type Cache interface {
	GetPair(ctx context.Context, key string) (*entities.Pair, error)
	SetPair(ctx context.Context, key string, pair *entities.Pair, ttl time.Duration) error
	GetPrice(ctx context.Context, key string) (string, error)
	SetPrice(ctx context.Context, key string, price string, ttl time.Duration) error
	Delete(ctx context.Context, key string) error
}

// RedisCache implements Cache using Redis
type RedisCache struct {
	client *redis.Client
}

// NewRedisCache creates a new Redis cache client
func NewRedisCache(addr, password string, db int) (*RedisCache, error) {
	client := redis.NewClient(&redis.Options{
		Addr:     addr,
		Password: password,
		DB:       db,
	})

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := client.Ping(ctx).Err(); err != nil {
		return nil, fmt.Errorf("failed to connect to redis: %w", err)
	}

	return &RedisCache{client: client}, nil
}

// Close closes the Redis connection
func (c *RedisCache) Close() error {
	return c.client.Close()
}

// GetPair retrieves a cached pair
func (c *RedisCache) GetPair(ctx context.Context, key string) (*entities.Pair, error) {
	data, err := c.client.Get(ctx, key).Bytes()
	if err != nil {
		if err == redis.Nil {
			return nil, nil // Cache miss
		}
		return nil, err
	}

	var pair entities.Pair
	if err := json.Unmarshal(data, &pair); err != nil {
		return nil, err
	}

	return &pair, nil
}

// SetPair caches a pair with TTL
func (c *RedisCache) SetPair(ctx context.Context, key string, pair *entities.Pair, ttl time.Duration) error {
	data, err := json.Marshal(pair)
	if err != nil {
		return err
	}

	return c.client.Set(ctx, key, data, ttl).Err()
}

// GetPrice retrieves a cached price
func (c *RedisCache) GetPrice(ctx context.Context, key string) (string, error) {
	price, err := c.client.Get(ctx, key).Result()
	if err != nil {
		if err == redis.Nil {
			return "", nil // Cache miss
		}
		return "", err
	}
	return price, nil
}

// SetPrice caches a price with TTL
func (c *RedisCache) SetPrice(ctx context.Context, key string, price string, ttl time.Duration) error {
	return c.client.Set(ctx, key, price, ttl).Err()
}

// Delete removes a key from cache
func (c *RedisCache) Delete(ctx context.Context, key string) error {
	return c.client.Del(ctx, key).Err()
}

// PairCacheKey generates a cache key for a pair
func PairCacheKey(dex entities.DEXType, token0, token1 string) string {
	return fmt.Sprintf("pair:%s:%s:%s", dex, token0, token1)
}

// PriceCacheKey generates a cache key for a price
func PriceCacheKey(token string) string {
	return fmt.Sprintf("price:%s", token)
}

// InMemoryCache implements Cache using in-memory storage (for testing/development)
type InMemoryCache struct {
	pairs  map[string]*cachedPair
	prices map[string]*cachedPrice
}

type cachedPair struct {
	pair      *entities.Pair
	expiresAt time.Time
}

type cachedPrice struct {
	price     string
	expiresAt time.Time
}

// NewInMemoryCache creates a new in-memory cache
func NewInMemoryCache() *InMemoryCache {
	return &InMemoryCache{
		pairs:  make(map[string]*cachedPair),
		prices: make(map[string]*cachedPrice),
	}
}

func (c *InMemoryCache) GetPair(ctx context.Context, key string) (*entities.Pair, error) {
	if cached, ok := c.pairs[key]; ok {
		if time.Now().Before(cached.expiresAt) {
			return cached.pair, nil
		}
		delete(c.pairs, key)
	}
	return nil, nil
}

func (c *InMemoryCache) SetPair(ctx context.Context, key string, pair *entities.Pair, ttl time.Duration) error {
	c.pairs[key] = &cachedPair{
		pair:      pair,
		expiresAt: time.Now().Add(ttl),
	}
	return nil
}

func (c *InMemoryCache) GetPrice(ctx context.Context, key string) (string, error) {
	if cached, ok := c.prices[key]; ok {
		if time.Now().Before(cached.expiresAt) {
			return cached.price, nil
		}
		delete(c.prices, key)
	}
	return "", nil
}

func (c *InMemoryCache) SetPrice(ctx context.Context, key string, price string, ttl time.Duration) error {
	c.prices[key] = &cachedPrice{
		price:     price,
		expiresAt: time.Now().Add(ttl),
	}
	return nil
}

func (c *InMemoryCache) Delete(ctx context.Context, key string) error {
	delete(c.pairs, key)
	delete(c.prices, key)
	return nil
}
