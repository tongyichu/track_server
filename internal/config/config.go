package config

import (
	"os"
)

// Config holds server configuration loaded from environment variables.
type Config struct {
	MongoURI        string
	MongoDBName     string
	MySQLDSN        string
	ServerAddr      string
	LogDir          string
	UseInMemory     bool
	UseMySQL        bool
	WechatAppID     string
	WechatAppSecret string
	AMapWebKey      string
	AMapRESTKey     string
	TLSCertFile     string
	TLSKeyFile      string
	EnableTLS       bool
	JWTSecret       string
}

// Load reads configuration from environment variables with sensible defaults.
func Load() *Config {
	tlsCert := os.Getenv("TLS_CERT_FILE")
	tlsKey := os.Getenv("TLS_KEY_FILE")

	// Backward compatibility:
	// - Some deployment examples used BIND_ADDR instead of SERVER_ADDR
	serverAddr := os.Getenv("SERVER_ADDR")
	if serverAddr == "" {
		serverAddr = getEnv("BIND_ADDR", ":8080")
	}

	// Backward compatibility:
	// - Some env files used AMAP_KEY instead of AMAP_WEB_KEY / AMAP_REST_KEY
	aMapWebKey := os.Getenv("AMAP_WEB_KEY")
	if aMapWebKey == "" {
		aMapWebKey = os.Getenv("AMAP_KEY")
	}
	aMapRestKey := os.Getenv("AMAP_REST_KEY")

	cfg := &Config{
		MongoURI:        getEnv("MONGO_URI", ""),
		MongoDBName:     getEnv("MONGO_DB_NAME", "trackapp"),
		MySQLDSN:        getEnv("MYSQL_DSN", ""),
		ServerAddr:      serverAddr,
		LogDir:          getEnv("LOG_DIR", "/var/log/track_server"),
		WechatAppID:     os.Getenv("WECHAT_APP_ID"),
		WechatAppSecret: os.Getenv("WECHAT_APP_SECRET"),
		AMapWebKey:      aMapWebKey,
		AMapRESTKey:     aMapRestKey,
		TLSCertFile:     tlsCert,
		TLSKeyFile:      tlsKey,
		EnableTLS:       tlsCert != "" && tlsKey != "",
		JWTSecret:       getEnv("JWT_SECRET", "track_server_default_jwt_secret"),
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
		// 内存兜底
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
