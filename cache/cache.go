package cache

import (
	"github.com/go-redis/redis/v8"
	"os"
	"strings"
)

func parseRedisUrl(redisUrl string) (string, string) {
	if redisUrl == "" {
		// local dev
		return "localhost:6379", ""
	}
	// e.g., redis://default:password@redis-1.2.3.machine.cloud.redislabs.com:port
	hostAddr := strings.Split(redisUrl, "@")[1]
	password := strings.Split(strings.Split(redisUrl, "@")[0], ":")[2]
	return hostAddr, password
}

func RedisClient() *redis.Client {
	herokuRedisURL := os.Getenv("REDISCLOUD_URL")  // set automatically by Heroku
	hostAddr, password := parseRedisUrl(herokuRedisURL)
	rdb := redis.NewClient(&redis.Options{
		Addr: hostAddr,
		Password: password,
		DB:       0,  // use default DB
		PoolSize: 5,  // max conns for Heroku free tier is 6
	})

	return rdb
}
