package config

import (
	"os"
)

// Config holds server configuration loaded from environment variables.
type Config struct {
	MongoURI       string
	MongoDBName    string
	MySQLDSN       string
	ServerAddr     string
	UseInMemory    bool
	UseMySQL       bool
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
		MySQLDSN:       getEnv("MYSQL_DSN", ""),
		ServerAddr:     getEnv("SERVER_ADDR", ":8080"),
		WechatAppID:    os.Getenv("WECHAT_APP_ID"),
		WechatAppSecret: os.Getenv("WECHAT_APP_SECRET"),
		AMapWebKey:     os.Getenv("AMAP_WEB_KEY"),
		AMapRESTKey:    os.Getenv("AMAP_REST_KEY"),
	}

	// Priority:
	// 1) USE_IN_MEMORY_STORE=true
	// 2) MYSQL_DSN is set
	// 3) MONGO_URI is set
	// 4) fallback to in-memory
	if os.Getenv("USE_IN_MEMORY_STORE") == "true" {
		cfg.UseInMemory = true
		return cfg
	}
	if cfg.MySQLDSN != "" {
		cfg.UseMySQL = true
		return cfg
	}
	if cfg.MongoURI == "" {
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
