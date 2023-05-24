package main

import (
	"fmt"
	"net/http"
	"os"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/go-redis/redis/v8"
)

type RedisClient struct {
	client *redis.Client
}

var (
	redisClient *RedisClient
)

const (
	redisAddress = "localhost:6379"
)

func GetRedisClient() *RedisClient {

	redisClient = &RedisClient{
		client: redis.NewClient(&redis.Options{
			Addr: redisAddress,
			DB:   0,
		}),
	}

	return redisClient
}

func (rc *RedisClient) RateLimitMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {

		// Get the IP address
		IPAddress := c.GetHeader("X-Real-Ip")
		if IPAddress == "" {
			IPAddress = c.GetHeader("X-Forwarded-For")
		}
		if IPAddress == "" {
			IPAddress = c.Request.RemoteAddr
		}
		fmt.Println(IPAddress)

		//getting the content of the lua script
		script, err := os.ReadFile("script.lua")

		if err != nil {

			c.AbortWithStatusJSON(http.StatusInternalServerError, gin.H{"status": false, "message": "unable to read script"})
		}

		var takeScript = redis.NewScript(string(script))
		const rate = 0.1    // 10 per second
		const capacity = 10 // burst of up to 10

		now := time.Now().UnixMicro()
		res, err := takeScript.Run(c, rc.client, []string{IPAddress}, capacity, rate, now, 1).Result()
		if err != nil {
			panic(err)
		}

		allowed := res.(int64)
		fmt.Println(allowed)
		if allowed != 1 {
			// request will be aborted
			c.AbortWithStatusJSON(http.StatusTooManyRequests, gin.H{"status": false, "message": "request overflowed"})
		}

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

//seq 1 200 | xargs -n1 -P10  curl "http://localhost:8080/ping"
