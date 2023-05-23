package main

import (
	"fmt"
	"net/http"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/go-redis/redis/v8"
)

	type RedisClient struct {
		client *redis.Client
	}

	var (
		redisClient *RedisClient
		once        sync.Once
	)

	const (
		redisAddress = "localhost:6379"
	)

	const (
		bucketSize  int64= 10 // Number of tokens in the bucket
		fillRate   = 1  // Tokens filled per second
	)

	func GetRedisClient() *RedisClient {
		
			redisClient = &RedisClient{
				client: redis.NewClient(&redis.Options{
					Addr: redisAddress,
					DB: 0,
				}),
			}

		return redisClient
	}

	func (rc *RedisClient) RateLimitMiddleware() gin.HandlerFunc {
		return func(c *gin.Context) {
			// Get the current timestamp in seconds
			currentTime := time.Now().Unix()/60

			// Get the IP address
			IPAddress := c.GetHeader("X-Real-Ip")
			if IPAddress == "" {
				IPAddress = c.GetHeader("X-Forwarded-For")
			}
			if IPAddress == "" {
				IPAddress = c.Request.RemoteAddr
			}
			fmt.Println(IPAddress)

			// Check if the key exists in Redis
			exists, err := rc.client.Exists(c, IPAddress).Result()
			if err != nil {
				c.AbortWithStatus(http.StatusInternalServerError)
				return
			}
			
			// If the key doesn't exist, set the initial count value
			if exists == 0 {
				err = rc.client.Set(c, IPAddress,"0", 0).Err()
				if err != nil {
					c.AbortWithStatus(http.StatusInternalServerError)
					return
				}
			}

			// Get the count value from Redis
			count, err := rc.client.Get(c, IPAddress).Int64()
			fmt.Println(count)
			if err != nil && err != redis.Nil {
				c.AbortWithStatus(http.StatusInternalServerError)
				return
			}


			// Calculate the tokens to be added to the bucket
			tokensToAdd := (currentTime - count) * fillRate
			fmt.Println(tokensToAdd)
			if tokensToAdd > 0 {
				// Add tokens to the bucket
				pipe := rc.client.TxPipeline()
				pipe.IncrBy(c, IPAddress, tokensToAdd)
				pipe.Exec(c)
			}
			
			// Check if the number of tokens in the bucket is sufficient
			var bucketSize int64=10
			if count >= bucketSize {
				c.AbortWithStatusJSON(http.StatusTooManyRequests, gin.H{
					"error": "Rate limit exceeded",
				})
				return
			}

			// Consume one token from the bucket
			rc.client.Decr(c, IPAddress)
		
			
			c.Next()
		}
	}

	func PingHandler(c *gin.Context) {
		c.String(http.StatusOK, "Pong")
	}

	func main() {
		redisClient := GetRedisClient()

		router := gin.Default()
		router.Use(redisClient.RateLimitMiddleware())

		router.GET("/ping", PingHandler)

		if err := router.Run(":8080"); err != nil {
			fmt.Println("Failed to start server:", err)
		}
	}
