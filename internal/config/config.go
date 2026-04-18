package config

import (
	"os"
)

// Config holds server configuration loaded from environment variables.
type Config struct {
	MongoURI       string
	MongoDBName    string
	ServerAddr     string
	UseInMemory    bool
	WechatAppID    string
	WechatAppSecret string
	AMapWebKey     string
	AMapRESTKey    string
}

// Load reads configuration from environment variables with sensible defaults.
func Load() *Config {
	cfg := &Config{
		MongoURI:       getEnv("MONGO_URI", ""),
		MongoDBName:    getEnv("MONGO_DB_NAME", "trackapp"),
		ServerAddr:     getEnv("SERVER_ADDR", ":8080"),
		WechatAppID:    os.Getenv("WECHAT_APP_ID"),
		WechatAppSecret: os.Getenv("WECHAT_APP_SECRET"),
		AMapWebKey:     os.Getenv("AMAP_WEB_KEY"),
		AMapRESTKey:    os.Getenv("AMAP_REST_KEY"),
	}

	// When Mongo URI is empty or USE_IN_MEMORY_STORE=true, use in-memory repositories.
	if cfg.MongoURI == "" || os.Getenv("USE_IN_MEMORY_STORE") == "true" {
		cfg.UseInMemory = true
	}

	return cfg
}

func getEnv(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}
