package config

import (
	"os"
	"strconv"
)

const (
	DefaultOSSPathPrefix      = "prod/track/"
	DefaultOSSBucket          = "track-resource"
	DefaultSTSRegion          = "cn-hangzhou"
	DefaultSTSDurationSeconds = 900
	DefaultRoleSessionPref    = "trackapp-"
	OSSFileBucketSize         = 2000 //所有用户的轨迹文件hash到2000个桶以内，该值不能轻易修改
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

	AliyunAccessKeyID     string // 服务端长期 AK（仅在服务端保存）。
	AliyunAccessKeySecret string // 服务端长期 SK（仅在服务端保存）。
	AliyunRoleARN         string // 用于 STS AssumeRole 的 RAM 角色 ARN，例如：acs:ram::xxxxxxxxx:role/oss-rw，该角色需要具备 OSS 写入权限（且建议在角色侧也限制到对应 bucket/前缀），与服务端下发的 policy 取交集
	AliyunSTSRegion       string // STS 服务所在地域（用于拼接 STS endpoint，通常为 cn-hangzhou）。
	AliyunSTSDurationSec  int64  // 临时凭证有效期（秒），服务端会在创建 STS Service 时把该值钳制到 900~3600 秒，避免请求失败。
	AliyunRoleSessionPref string // RoleSessionName 前缀（云侧审计可见）。
	OSSBucket             string // 允许上传的目标 Bucket。该值会写入 STS policy 的 Resource（acs:oss:*:*:<bucket>/<prefix>*），必须与实际 bucket 一致。
	OSSRegion             string // Bucket 所在地域（主要用于客户端直传时选择配置，服务端申请 STS 不依赖该字段）。
	OSSEndpoint           string // Bucket 的访问 Endpoint（公网），会回传给客户端直传使用。示例：https://oss-cn-beijing.aliyuncs.com
	OSSInternalEndpoint   string // Bucket 的访问 Endpoint（内网），仅服务端从 OSS 拉取对象时使用，避免公网下行流量费用。示例：https://oss-cn-beijing-internal.aliyuncs.com
	OSSUploadPrefix       string // 服务端为“每个用户”分配的上传目录前缀。会在服务端生成最终目录：<prefix>/<userID>/，示例 OSS_UPLOAD_PREFIX=/prod/track/user  => dir=prod/track/user/1001/
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

		// Aliyun OSS STS（客户端直传上传凭证）
		// - ALIYUN_ACCESS_KEY_ID / ALIYUN_ACCESS_KEY_SECRET：服务端长期 AK/SK
		// - ALIYUN_ROLE_ARN：AssumeRole 的目标角色
		// - ALIYUN_STS_REGION：STS 服务地域
		// - ALIYUN_STS_DURATION_SECONDS：临时凭证时长（秒，最终会被钳制到 900~3600）
		// - ALIYUN_ROLE_SESSION_NAME_PREFIX：RoleSessionName 前缀
		AliyunAccessKeyID:     os.Getenv("ALIYUN_ACCESS_KEY_ID"),
		AliyunAccessKeySecret: os.Getenv("ALIYUN_ACCESS_KEY_SECRET"),
		AliyunRoleARN:         os.Getenv("ALIYUN_ROLE_ARN"),
		AliyunSTSRegion:       getEnv("ALIYUN_STS_REGION", DefaultSTSRegion),
		AliyunSTSDurationSec:  getEnvInt64("ALIYUN_STS_DURATION_SECONDS", DefaultSTSDurationSeconds),
		AliyunRoleSessionPref: getEnv("ALIYUN_ROLE_SESSION_NAME_PREFIX", DefaultRoleSessionPref),

		// OSS 侧直传上下文
		// - OSS_BUCKET：目标 bucket（会进入 STS policy 的 Resource）
		// - OSS_REGION/OSS_ENDPOINT：客户端直传时常用的区域/Endpoint 信息
		// - OSS_UPLOAD_PREFIX：用户目录前缀（最终为 <prefix>/<userID>/）
		OSSBucket:           getEnv("OSS_BUCKET", DefaultOSSBucket),
		OSSRegion:           getEnv("OSS_REGION", ""),
		OSSEndpoint:         getEnv("OSS_ENDPOINT", ""),
		OSSInternalEndpoint: getEnv("OSS_INTERNAL_ENDPOINT", ""),
		OSSUploadPrefix:     getEnv("OSS_UPLOAD_PREFIX", DefaultOSSPathPrefix),
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
