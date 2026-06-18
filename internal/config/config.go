package config

import (
	_ "embed"
	"encoding/json"
	"os"
	"strconv"
	"strings"
	"sync"
)

const (
	DefaultOSSPathPrefix               = "prod/track/"
	DefaultAnalyticsOSSPrefix          = "analytics/ods/"
	DefaultOSSBucket                   = "track-resource"
	DefaultSTSRegion                   = "cn-hangzhou"
	DefaultSTSDurationSeconds          = 900
	DefaultRoleSessionPref             = "trackapp-"
	DefaultCompanionMQTTTopic          = "companion"
	DefaultCompanionMQTTTTL            = 3600
	DefaultCompanionMQTTPublishTimeout = 5
	OSSFileBucketSize                  = 2000 //所有用户的轨迹文件hash到2000个桶以内，该值不能轻易修改
)

// TrackTypeConfig defines one built-in track type and its presentation metadata.
type TrackTypeConfig struct {
	Type         string
	Name         string
	ThemeColor   string
	IconFile     string
	IconAnimFile string
}

// DefaultTrackTypeConfigs keeps built-in track type names and icons in one place.
var DefaultTrackTypeConfigs = []TrackTypeConfig{
	{Type: "hiking", Name: "徒步", ThemeColor: "#345631", IconFile: "hiking.svg", IconAnimFile: ""},
	{Type: "running", Name: "跑步", ThemeColor: "#F26A4B", IconFile: "running.svg", IconAnimFile: ""},
	{Type: "climbing", Name: "爬山", ThemeColor: "#6C4CE1", IconFile: "climbing.svg", IconAnimFile: ""},
	{Type: "riding", Name: "骑行", ThemeColor: "#2F80ED", IconFile: "riding.svg", IconAnimFile: ""},
	{Type: "driving", Name: "自驾", ThemeColor: "#F5A623", IconFile: "driving.svg", IconAnimFile: ""},
}

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
	TrackTypes      []string // 客户端可选运动类型列表，默认：徒步/跑步/爬山/骑行/自驾。

	EMQXBrokerURL                    string
	EMQXWebsocketURL                 string
	EMQXClientBrokerURL              string
	EMQXClientWebsocketURL           string
	CompanionMQTTTopicPrefix         string
	CompanionMQTTCredentialTTLSecond int64
	CompanionMQTTCredentialSecret    string
	CompanionMQTTInternalToken       string
	OpsInternalToken                 string
	CompanionMQTTPublisherClientID   string
	CompanionMQTTPublisherUsername   string
	CompanionMQTTPublisherPassword   string
	CompanionMQTTPublishTimeoutSec   int64

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

	// 管理后台登录凭证（独立于业务用户的鉴权体系）
	//
	// 推荐：通过 ADMIN_ACCOUNTS 同时配置多个管理员，格式：
	//     user1:bcryptHash1;user2:bcryptHash2
	//   - 多个账号用 ";" 分隔；用户名与哈希用 ":" 分隔；
	//   - 哈希里本身可能含 ":"（bcrypt 不会，但保持稳健），username 只截取首个 ":" 之前。
	//
	// 兼容：仍支持旧的单账号配置：
	//   - ADMIN_USERNAME / ADMIN_PASSWORD_HASH
	//   - 若同时配置 ADMIN_ACCOUNTS 和 ADMIN_USERNAME，会合并；同名用户以 ADMIN_ACCOUNTS 为准。
	//
	// 任一来源解析为非空账号集时后台启用；否则后台禁用。
	// 哈希生成示例：htpasswd -nbBC 10 "" "<plain>" | tr -d ':\n' | sed 's/^$2y/$2a/'
	AdminAccounts map[string]string

	// 定时任务调度器（基于 robfig/cron/v3）
	// - SCHEDULER_ENABLED=true 时启动，便于后续把 API 集群与定时任务集群拆开部署；
	// - DANMAKU_CLEANUP_CRON：弹幕清理任务的 cron 表达式（5 段，默认每天 03:00）；
	// - DANMAKU_RETENTION_DAYS：会话结束后弹幕保留天数（默认 7 天）；
	// - TRACK_MAP_INDEX_CRON：轨迹地图索引任务的 cron 表达式（默认每 1 分钟）。
	// - TRACK_ROUTE_GROUP_CRON：路线组离线聚合任务的 cron 表达式（默认每天 04:00）。
	SchedulerEnabled     bool
	DanmakuCleanupCron   string
	DanmakuRetentionDays int
	TrackMapIndexCron    string
	TrackRouteGroupCron  string

	// 客户端埋点采集
	// - ANALYTICS_ENABLED=false 时关闭 /api/v1/analytics/events；
	// - ANALYTICS_LOCAL_DIR 为空时使用 <LogDir>/analytics/events；
	// - ANALYTICS_SYNC_CRON：本地埋点文件同步 OSS 的 cron 表达式（默认每天 03:00）；
	// - ANALYTICS_OSS_PREFIX：OSS ODS 归档前缀，最终路径会追加 event_date/hour 分区；
	// - ANALYTICS_MAX_BATCH_SIZE：单次上报事件条数上限（默认 50）；
	// - ANALYTICS_MAX_BODY_BYTES：单次请求 body 大小上限（默认 256 KiB）。
	AnalyticsEnabled      bool
	AnalyticsLocalDir     string
	AnalyticsSyncCron     string
	AnalyticsOSSPrefix    string
	AnalyticsMaxBatchSize int
	AnalyticsMaxBodyBytes int64
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
		MongoURI:                         getEnv("MONGO_URI", ""),
		MongoDBName:                      getEnv("MONGO_DB_NAME", "trackapp"),
		MySQLDSN:                         getEnv("MYSQL_DSN", ""),
		ServerAddr:                       serverAddr,
		LogDir:                           getEnv("LOG_DIR", "/var/log/track_server"),
		WechatAppID:                      os.Getenv("WECHAT_APP_ID"),
		WechatAppSecret:                  os.Getenv("WECHAT_APP_SECRET"),
		AMapWebKey:                       aMapWebKey,
		AMapRESTKey:                      aMapRestKey,
		TLSCertFile:                      tlsCert,
		TLSKeyFile:                       tlsKey,
		EnableTLS:                        tlsCert != "" && tlsKey != "",
		JWTSecret:                        getEnv("JWT_SECRET", "track_server_default_jwt_secret"),
		TrackTypes:                       ParseTrackTypes(os.Getenv("TRACK_TYPES")),
		EMQXBrokerURL:                    getEnv("EMQX_BROKER_URL", ""),
		EMQXWebsocketURL:                 getEnv("EMQX_WEBSOCKET_URL", ""),
		EMQXClientBrokerURL:              getEnv("EMQX_CLIENT_BROKER_URL", ""),
		EMQXClientWebsocketURL:           getEnv("EMQX_CLIENT_WEBSOCKET_URL", ""),
		CompanionMQTTTopicPrefix:         getEnv("COMPANION_MQTT_TOPIC_PREFIX", DefaultCompanionMQTTTopic),
		CompanionMQTTCredentialTTLSecond: getEnvInt64("COMPANION_MQTT_CREDENTIAL_TTL_SECONDS", DefaultCompanionMQTTTTL),
		CompanionMQTTCredentialSecret:    os.Getenv("COMPANION_MQTT_CREDENTIAL_SECRET"),
		CompanionMQTTInternalToken:       os.Getenv("COMPANION_MQTT_INTERNAL_TOKEN"),
		OpsInternalToken:                 os.Getenv("OPS_INTERNAL_TOKEN"),
		CompanionMQTTPublisherClientID:   getEnv("COMPANION_MQTT_PUBLISHER_CLIENT_ID", "track-server-companion-publisher"),
		CompanionMQTTPublisherUsername:   os.Getenv("COMPANION_MQTT_PUBLISHER_USERNAME"),
		CompanionMQTTPublisherPassword:   os.Getenv("COMPANION_MQTT_PUBLISHER_PASSWORD"),
		CompanionMQTTPublishTimeoutSec:   getEnvInt64("COMPANION_MQTT_PUBLISH_TIMEOUT_SECONDS", DefaultCompanionMQTTPublishTimeout),

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

		// 管理后台登录凭证（独立于业务用户鉴权）
		AdminAccounts: parseAdminAccounts(os.Getenv("ADMIN_ACCOUNTS"), os.Getenv("ADMIN_USERNAME"), os.Getenv("ADMIN_PASSWORD_HASH")),

		// 定时任务调度器
		SchedulerEnabled:     os.Getenv("SCHEDULER_ENABLED") == "true",
		DanmakuCleanupCron:   getEnv("DANMAKU_CLEANUP_CRON", "0 3 * * *"),
		DanmakuRetentionDays: int(getEnvInt64("DANMAKU_RETENTION_DAYS", 7)),
		TrackMapIndexCron:    getEnv("TRACK_MAP_INDEX_CRON", "@every 10m"),
		TrackRouteGroupCron:  getEnv("TRACK_ROUTE_GROUP_CRON", "0 4 * * *"),

		// 客户端埋点采集
		AnalyticsEnabled:      getEnv("ANALYTICS_ENABLED", "true") != "false",
		AnalyticsLocalDir:     os.Getenv("ANALYTICS_LOCAL_DIR"),
		AnalyticsSyncCron:     getEnv("ANALYTICS_SYNC_CRON", "0 3 * * *"),
		AnalyticsOSSPrefix:    getEnv("ANALYTICS_OSS_PREFIX", DefaultAnalyticsOSSPrefix),
		AnalyticsMaxBatchSize: int(getEnvInt64("ANALYTICS_MAX_BATCH_SIZE", 50)),
		AnalyticsMaxBodyBytes: getEnvInt64("ANALYTICS_MAX_BODY_BYTES", 256*1024),
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

// ParseTrackTypes parses configurable track types from env value.
// Supported separators: comma, semicolon, Chinese comma/dunhao and vertical bar.
// Empty input or all-empty entries fall back to DefaultTrackTypes().
func ParseTrackTypes(raw string) []string {
	items := splitByAny(raw, ",;，、|")
	if len(items) == 0 {
		return DefaultTrackTypes()
	}
	out := make([]string, 0, len(items))
	seen := make(map[string]struct{}, len(items))
	for _, item := range items {
		item = strings.TrimSpace(item)
		if item == "" {
			continue
		}
		if _, ok := seen[item]; ok {
			continue
		}
		seen[item] = struct{}{}
		out = append(out, item)
	}
	if len(out) == 0 {
		return DefaultTrackTypes()
	}
	return out
}

// DefaultTrackTypes returns built-in track type names in display order.
func DefaultTrackTypes() []string {
	out := make([]string, 0, len(DefaultTrackTypeConfigs))
	for _, item := range DefaultTrackTypeConfigs {
		out = append(out, item.Name)
	}
	return out
}

// TrackTypeIconFile returns built-in icon filename for a track type.
func TrackTypeIconFile(trackType string) (string, bool) {
	for _, item := range DefaultTrackTypeConfigs {
		if item.Name == trackType {
			return item.IconFile, true
		}
	}
	return "", false
}

// TrackTypeConfigByName returns built-in type metadata by display name.
func TrackTypeConfigByName(trackType string) (TrackTypeConfig, bool) {
	for _, item := range DefaultTrackTypeConfigs {
		if item.Name == trackType {
			return item, true
		}
	}
	return TrackTypeConfig{}, false
}

// parseAdminAccounts 解析管理员账号配置，返回 username -> bcryptHash 的映射。
//
// 解析优先级：
//  1. 先解析 ADMIN_ACCOUNTS（格式：user1:hash1;user2:hash2，支持 "," 作为分隔符）；
//  2. 再合并旧的 ADMIN_USERNAME / ADMIN_PASSWORD_HASH（若两者都非空）；
//     — 同名用户以 ADMIN_ACCOUNTS 中的为准（先写入者保留）。
//
// 规则：
//   - 每个条目 split 第一个 ":"：前半为 username，后半为 hash（hash 内即使含 ":" 也保留原样）；
//   - username/hash 都会 TrimSpace；任一为空的条目忽略；
//   - 返回空 map 表示后台禁用。
func parseAdminAccounts(rawAccounts, legacyUser, legacyHash string) map[string]string {
	out := make(map[string]string)

	for _, entry := range splitByAny(rawAccounts, ";,") {
		entry = strings.TrimSpace(entry)
		if entry == "" {
			continue
		}
		idx := strings.IndexByte(entry, ':')
		if idx <= 0 || idx == len(entry)-1 {
			continue
		}
		user := strings.TrimSpace(entry[:idx])
		hash := strings.TrimSpace(entry[idx+1:])
		if user == "" || hash == "" {
			continue
		}
		if _, exists := out[user]; !exists {
			out[user] = hash
		}
	}

	legacyUser = strings.TrimSpace(legacyUser)
	legacyHash = strings.TrimSpace(legacyHash)
	if legacyUser != "" && legacyHash != "" {
		if _, exists := out[legacyUser]; !exists {
			out[legacyUser] = legacyHash
		}
	}

	return out
}

// splitByAny 按 seps 中任一字符拆分 s；seps 为空时退化为单字符 ";" 切分。
func splitByAny(s, seps string) []string {
	if s == "" {
		return nil
	}
	if seps == "" {
		seps = ";"
	}
	return strings.FieldsFunc(s, func(r rune) bool {
		return strings.ContainsRune(seps, r)
	})
}

// -----------------------------
// 城市/省份配置（内置 JSON）
// -----------------------------
//
// 为什么把 JSON 放在 internal/config 并用 go:embed 内置：
// - 这类映射属于“随服务版本发布的业务配置”，不是运行时环境变量；内置可以减少部署/挂载复杂度。
// - 列表接口需要在每条 TrackSummary 返回 city_name，读取配置必须足够快且不能成为单点故障。
// - 通过 sync.Once 在进程内完成一次性解析，后续查询为 map O(1)。
//
// 注意：
// - city_code 优先使用城市映射；直辖市、港澳台等省级 code 使用省份映射兜底。

//go:embed china_city_raw.json
var chinaCityRaw []byte

//go:embed china_province_raw.json
var chinaProvinceRaw []byte

type rawCity struct {
	Code     string `json:"code"`
	Name     string `json:"name"`
	Province string `json:"province"`
	City     string `json:"city"`
}

type rawProvince struct {
	Code     string `json:"code"`
	Name     string `json:"name"`
	Province string `json:"province"`
}

var (
	cityNameOnce sync.Once
	cityNameMap  map[string]string
)

// CityNameByCode 根据城市 Code 返回城市名称。
//
// - 数据来源：internal/config/china_city_raw.json 与 china_province_raw.json（编译期内置），由业务侧维护映射关系；
// - 城市表优先，省份表只用于 110000/310000/810000 等省级或直辖市 code 兜底；
// - 若 code 为空或未找到映射，返回空字符串；
// - 该函数不会抛错，避免影响推荐/搜索列表主流程（拿不到 city_name 时仍返回 city_code 以及其他字段）。
//
// 性能说明：
// - 首次调用会触发一次 JSON Unmarshal（仅一次）；
// - 后续每次调用为一次 map 查表；
// - city_code 的合法性校验不在此处做（写入时由上游保证/或后续增加校验逻辑）。
func CityNameByCode(code string) string {
	if code == "" {
		return ""
	}
	cityNameOnce.Do(func() {
		cityNameMap = make(map[string]string, 2500)
		var cities []rawCity
		if err := json.Unmarshal(chinaCityRaw, &cities); err != nil {
			cities = nil
		}
		for _, c := range cities {
			if c.Code == "" {
				continue
			}
			cityNameMap[c.Code] = c.Name
		}
		var provinces []rawProvince
		if err := json.Unmarshal(chinaProvinceRaw, &provinces); err != nil {
			return
		}
		for _, p := range provinces {
			if p.Code == "" {
				continue
			}
			if _, exists := cityNameMap[p.Code]; exists {
				continue
			}
			cityNameMap[p.Code] = p.Name
		}
	})
	return cityNameMap[code]
}
