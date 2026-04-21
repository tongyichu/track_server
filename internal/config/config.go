package config

import (
	"os"
	"strconv"
)

// Config holds server configuration loaded from environment variables.
type Config struct {
	MongoURI        string
	MongoDBName     string
	MySQLDSN        string // mysql数据库连接，示例 root:$密码@tcp(172.18.0.1:3306)/track_db?charset=utf8mb4&parseTime=True
	ServerAddr      string
	LogDir          string
	UseInMemory     bool
	UseMySQL        bool // 存储是否使用mysql
	WechatAppID     string
	WechatAppSecret string
	AMapWebKey      string
	AMapRESTKey     string
	TLSCertFile     string
	TLSKeyFile      string
	EnableTLS       bool
	JWTSecret       string

	// Aliyun OSS STS (temporary credentials for direct upload)
	AliyunAccessKeyID     string
	AliyunAccessKeySecret string
	AliyunRoleARN         string
	AliyunSTSRegion       string
	AliyunSTSDurationSec  int64
	AliyunRoleSessionPref string

	OSSBucket       string
	OSSRegion       string
	OSSEndpoint     string
	OSSUploadPrefix string
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

		AliyunAccessKeyID:     os.Getenv("ALIYUN_ACCESS_KEY_ID"),
		AliyunAccessKeySecret: os.Getenv("ALIYUN_ACCESS_KEY_SECRET"),
		AliyunRoleARN:         os.Getenv("ALIYUN_ROLE_ARN"),
		AliyunSTSRegion:       getEnv("ALIYUN_STS_REGION", "cn-hangzhou"),
		AliyunSTSDurationSec:  getEnvInt64("ALIYUN_STS_DURATION_SECONDS", 900),
		AliyunRoleSessionPref: getEnv("ALIYUN_ROLE_SESSION_NAME_PREFIX", "trackapp-"),

		OSSBucket:       os.Getenv("OSS_BUCKET"),
		OSSRegion:       getEnv("OSS_REGION", ""),
		OSSEndpoint:     getEnv("OSS_ENDPOINT", ""),
		OSSUploadPrefix: getEnv("OSS_UPLOAD_PREFIX", "user"),
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

func getEnvInt64(key string, def int64) int64 {
	v := os.Getenv(key)
	if v == "" {
		return def
	}
	// strconv.ParseInt treats leading/trailing spaces as invalid; keep strict.
	if n, err := strconv.ParseInt(v, 10, 64); err == nil {
		return n
	}
	return def
}
