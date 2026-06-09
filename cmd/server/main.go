package main

import (
	"context"
	"crypto/tls"
	"io"
	"log"
	"os"
	"path/filepath"
	"time"

	"github.com/cloudwego/hertz/pkg/app/server"
	"github.com/cloudwego/hertz/pkg/common/hlog"

	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"

	"github.com/tongyichu/track_server/internal/admin"
	"github.com/tongyichu/track_server/internal/config"
	"github.com/tongyichu/track_server/internal/handler"
	"github.com/tongyichu/track_server/internal/middleware"
	"github.com/tongyichu/track_server/internal/repository"
	"github.com/tongyichu/track_server/internal/scheduler"
	"github.com/tongyichu/track_server/internal/scheduler/jobs"
	"github.com/tongyichu/track_server/internal/service"
)

const maxRequestBodySize = 100 * 1024 * 1024 // 100 MiB，需覆盖管理后台安装包 multipart 上传

// main is the entrypoint of the Hertz HTTP server.
func main() {
	cfg := config.Load()

	logFile, err := setupLogging(cfg.LogDir)
	if err != nil {
		log.Printf("failed to initialize file logging in %s, fallback to stdout/stderr only: %v", cfg.LogDir, err)
	} else {
		defer func() {
			_ = logFile.Close()
		}()
		log.Printf("file logging enabled: %s", logFile.Name())
	}

	ctx := context.Background()

	var trackRepo repository.TrackRepository
	var userRepo repository.UserRepository
	var collectRepo repository.CollectRepository
	var loginLogRepo repository.LoginLogRepository
	var navigationRepo repository.NavigationRepository
	var appReleaseRepo repository.AppReleaseRepository
	var companionRepo repository.CompanionRepository
	var achievementRepo repository.AchievementRepository

	if cfg.UseInMemory {
		trackRepo, userRepo, collectRepo, loginLogRepo, navigationRepo, appReleaseRepo, companionRepo = repository.NewInMemoryRepositories()
		achievementRepo = repository.NewInMemoryAchievementRepository()
		log.Println("using in-memory repositories")
	} else if cfg.UseMySQL {
		db, err := repository.OpenMySQL(cfg.MySQLDSN)
		if err != nil {
			log.Printf("failed to open mysql, fallback to memory: %v", err)
			trackRepo, userRepo, collectRepo, loginLogRepo, navigationRepo, appReleaseRepo, companionRepo = repository.NewInMemoryRepositories()
			achievementRepo = repository.NewInMemoryAchievementRepository()
		} else if err := db.PingContext(ctx); err != nil {
			log.Printf("failed to ping mysql, fallback to memory: %v", err)
			_ = db.Close()
			trackRepo, userRepo, collectRepo, loginLogRepo, navigationRepo, appReleaseRepo, companionRepo = repository.NewInMemoryRepositories()
			achievementRepo = repository.NewInMemoryAchievementRepository()
		} else {
			tr, ur, cr, lr, nr, ar, cor, err := repository.NewMySQLRepositories(ctx, db)
			if err != nil {
				log.Printf("failed to init mysql schema, fallback to memory: %v", err)
				_ = db.Close()
				trackRepo, userRepo, collectRepo, loginLogRepo, navigationRepo, appReleaseRepo, companionRepo = repository.NewInMemoryRepositories()
				achievementRepo = repository.NewInMemoryAchievementRepository()
			} else {
				defer func() {
					_ = db.Close()
				}()
				trackRepo, userRepo, collectRepo, loginLogRepo, navigationRepo, appReleaseRepo, companionRepo = tr, ur, cr, lr, nr, ar, cor
				achievementRepo = repository.NewMySQLAchievementRepository(db)
				log.Println("using mysql repositories")
			}
		}
	} else {
		log.Println("using mongo repositories")
		client, err := mongo.Connect(ctx, options.Client().ApplyURI(cfg.MongoURI))
		if err != nil {
			log.Printf("failed to connect mongo, fallback to memory: %v", err)
			trackRepo, userRepo, collectRepo, loginLogRepo, navigationRepo, appReleaseRepo, companionRepo = repository.NewInMemoryRepositories()
			achievementRepo = repository.NewInMemoryAchievementRepository()
		} else {
			db := client.Database(cfg.MongoDBName)
			trackRepo = repository.NewMongoTrackRepository(db.Collection("tracks"))
			userRepo = repository.NewMongoUserRepository(db.Collection("users"))
			collectRepo = repository.NewMongoCollectRepository(db.Collection("track_collects"))
			loginLogRepo = repository.NewMongoLoginLogRepository(db.Collection("login_log"))
			navigationRepo = repository.NewMongoNavigationRepository(db.Collection("track_navigations"))
			appReleaseRepo = repository.NewMongoAppReleaseRepository(db.Collection("app_releases"))
			achievementRepo = repository.NewMongoAchievementRepository(db.Collection("user_achievement_rewards"))
			companionRepo = repository.NewMongoCompanionRepository(
				db.Collection("companion_sessions"),
				db.Collection("companion_session_members"),
				db.Collection("companion_live_positions"),
			)
			log.Println("mongo repositories initialized")
		}
	}

	trackSvc := service.NewTrackService(trackRepo, collectRepo)
	trackSvc.SetTrackTypes(cfg.TrackTypes)
	trackSvc.SetUserRepository(userRepo)
	trackSvc.SetNavigationRepository(navigationRepo)
	trackSvc.SetCompanionRepository(companionRepo)
	achievementSvc := service.NewAchievementService(achievementRepo, trackRepo)
	trackSvc.SetAchievementService(achievementSvc)
	userSvc := service.NewUserService(userRepo)
	userSvc.SetTrackRepository(trackRepo)
	userSvc.SetNavigationRepository(navigationRepo)
	loginSvc := service.NewLoginService(userRepo, loginLogRepo, cfg.WechatAppID, cfg.WechatAppSecret, cfg.JWTSecret)
	appReleaseSvc := service.NewAppReleaseService(appReleaseRepo)
	companionSvc := service.NewCompanionService(companionRepo, userRepo)
	companionSvc.SetTrackRepository(trackRepo)
	clientBrokerURL := cfg.EMQXClientBrokerURL
	if clientBrokerURL == "" {
		clientBrokerURL = cfg.EMQXBrokerURL
	}
	clientWebsocketURL := cfg.EMQXClientWebsocketURL
	if clientWebsocketURL == "" {
		clientWebsocketURL = cfg.EMQXWebsocketURL
	}
	companionSvc.SetMQTTOptions(service.CompanionMQTTOptions{
		BrokerURL:        clientBrokerURL,
		WebsocketURL:     clientWebsocketURL,
		TopicPrefix:      cfg.CompanionMQTTTopicPrefix,
		CredentialTTL:    time.Duration(cfg.CompanionMQTTCredentialTTLSecond) * time.Second,
		CredentialSecret: cfg.CompanionMQTTCredentialSecret,
	})
	if cfg.EMQXBrokerURL != "" || cfg.EMQXWebsocketURL != "" {
		publisherBrokerURL := cfg.EMQXBrokerURL
		if publisherBrokerURL == "" {
			publisherBrokerURL = cfg.EMQXWebsocketURL
		}
		controlPublisher, err := service.NewMQTTCompanionControlPublisher(service.CompanionMQTTControlPublisherOptions{
			BrokerURL:      publisherBrokerURL,
			ClientID:       cfg.CompanionMQTTPublisherClientID,
			Username:       cfg.CompanionMQTTPublisherUsername,
			Password:       cfg.CompanionMQTTPublisherPassword,
			PublishTimeout: time.Duration(cfg.CompanionMQTTPublishTimeoutSec) * time.Second,
		})
		if err != nil {
			log.Printf("companion control publisher disabled: %v", err)
		} else {
			companionSvc.SetControlPublisher(controlPublisher)
			defer func() {
				_ = controlPublisher.Close()
			}()
			log.Printf("companion control publisher enabled: %s", publisherBrokerURL)
		}
	}

	// 初始化本地资源缓存服务：把客户端上传到 OSS 的轨迹截图 / 原始轨迹文件，按需同步到服务器本地，
	// 供列表/详情接口返回一个服务端可直接下载的 URL。
	// 缓存目录按类别分子目录放在 <LogDir>/static/<category>，统一通过 /static 静态路由下发。
	staticRoot := filepath.Join(cfg.LogDir, "static")
	screenshotCacheDir := filepath.Join(staticRoot, "screenshots")
	screenshotCache, err := service.NewAssetCacheService(
		screenshotCacheDir,
		"/api/v1/static/screenshots",
		[]string{".png", ".jpg", ".jpeg", ".webp", ".svg"},
		".png",
	)
	if err != nil {
		log.Printf("screenshot cache disabled: %v", err)
		screenshotCache = nil
	} else {
		trackSvc.SetScreenshotCache(screenshotCache)
		companionSvc.SetScreenshotCache(screenshotCache)
		log.Printf("screenshot cache enabled: %s", screenshotCacheDir)
	}

	avatarCacheDir := filepath.Join(staticRoot, "avatars")
	avatarCache, err := service.NewAssetCacheService(
		avatarCacheDir,
		"/api/v1/static/avatars",
		[]string{".png", ".jpg", ".jpeg", ".webp"},
		".png",
	)
	if err != nil {
		log.Printf("avatar cache disabled: %v", err)
		avatarCache = nil
	} else {
		trackSvc.SetAvatarCache(avatarCache)
		userSvc.SetAvatarCache(avatarCache)
		companionSvc.SetAvatarCache(avatarCache)
		log.Printf("avatar cache enabled: %s", avatarCacheDir)
	}

	rawTrackCacheDir := filepath.Join(staticRoot, "raw_tracks")
	// 原始轨迹文件后缀未作强约束（客户端可能用 .dat/.json/.gpx/.kmz 等），这里允许常见几种，
	// 未识别时按 .dat 落盘；文件名仍以 track_id 为主键保证唯一。
	rawTrackCache, err := service.NewAssetCacheService(
		rawTrackCacheDir,
		"/api/v1/static/raw_tracks",
		[]string{".dat", ".json", ".gpx", ".kmz", ".zip"},
		".dat",
	)
	if err != nil {
		log.Printf("raw track cache disabled: %v", err)
		rawTrackCache = nil
	} else {
		trackSvc.SetRawTrackCache(rawTrackCache)
		log.Printf("raw track cache enabled: %s", rawTrackCacheDir)
	}

	// Aliyun OSS STS（用于客户端直传）
	// 相关启动参数在 internal/config/config.go 中通过环境变量加载：
	// - ALIYUN_ACCESS_KEY_ID / ALIYUN_ACCESS_KEY_SECRET / ALIYUN_ROLE_ARN / ALIYUN_STS_REGION
	// - ALIYUN_STS_DURATION_SECONDS / ALIYUN_ROLE_SESSION_NAME_PREFIX
	// - OSS_BUCKET / OSS_REGION / OSS_ENDPOINT / OSS_UPLOAD_PREFIX
	// 若缺少关键配置，服务会降级为“禁用 STS”，对应接口会返回 "oss sts not configured"。
	ossTokenSvc, err := service.NewOSSTokenService(
		cfg.AliyunSTSRegion,
		cfg.AliyunAccessKeyID,
		cfg.AliyunAccessKeySecret,
		cfg.AliyunRoleARN,
		cfg.AliyunSTSDurationSec,
		cfg.AliyunRoleSessionPref,
		cfg.OSSBucket,
		cfg.OSSRegion,
		cfg.OSSEndpoint,
		cfg.OSSInternalEndpoint,
		cfg.OSSUploadPrefix,
	)
	if err != nil {
		log.Printf("oss sts disabled: %v", err)
		ossTokenSvc = nil
	}

	// 把 OSS 下载能力注入资源缓存服务。OSS 对象带权限控制，无法直接 http.Get，
	// 因此必须通过 STS 临时凭证 + OSS SDK 的方式下载。
	if ossTokenSvc != nil {
		if avatarCache != nil {
			avatarCache.SetDownloader(ossTokenSvc)
		}
		if screenshotCache != nil {
			screenshotCache.SetDownloader(ossTokenSvc)
		}
		if rawTrackCache != nil {
			rawTrackCache.SetDownloader(ossTokenSvc)
		}
	} else {
		if avatarCache != nil || screenshotCache != nil || rawTrackCache != nil {
			log.Printf("asset cache has no OSS downloader; only serves already-cached files")
		}
	}

	var h *server.Hertz
	if cfg.EnableTLS {
		cert, err := tls.LoadX509KeyPair(cfg.TLSCertFile, cfg.TLSKeyFile)
		if err != nil {
			log.Fatalf("failed to load TLS certificate: %v", err)
		}
		tlsCfg := &tls.Config{
			Certificates: []tls.Certificate{cert},
			MinVersion:   tls.VersionTLS12,
		}
		h = server.Default(
			server.WithHostPorts(cfg.ServerAddr),
			server.WithTLS(tlsCfg),
			server.WithMaxRequestBodySize(maxRequestBodySize),
		)
		log.Printf("server listening on %s (HTTPS)", cfg.ServerAddr)
	} else {
		h = server.Default(
			server.WithHostPorts(cfg.ServerAddr),
			server.WithMaxRequestBodySize(maxRequestBodySize),
		)
		log.Printf("server listening on %s (HTTP)", cfg.ServerAddr)
	}

	tokenBlacklist := middleware.NewTokenBlacklist()
	defer tokenBlacklist.Close()

	handler.RegisterRoutes(h, handler.Deps{
		TrackService:               trackSvc,
		UserService:                userSvc,
		LoginService:               loginSvc,
		OSSTokenService:            ossTokenSvc,
		AppReleaseService:          appReleaseSvc,
		CompanionService:           companionSvc,
		AchievementService:         achievementSvc,
		JWTSecret:                  cfg.JWTSecret,
		TokenBlacklist:             tokenBlacklist,
		CompanionMQTTInternalToken: cfg.CompanionMQTTInternalToken,
		OpsInternalToken:           cfg.OpsInternalToken,
		StaticRoot:                 staticRoot,
	})

	// 管理后台（独立于业务用户鉴权）。若未配置任何管理员账号，
	// NewModule 创建出的 auth 为 nil，RegisterRoutes 会直接跳过。
	adminModule := admin.NewModule(cfg.AdminAccounts, appReleaseSvc, ossTokenSvc, staticRoot, userRepo, trackRepo, companionRepo, userSvc)
	defer adminModule.Close()
	adminModule.RegisterRoutes(h)
	if adminModule != nil && adminModule.Auth != nil {
		log.Printf("admin console enabled at /admin/ (%d account(s))", adminModule.Auth.AccountCount())
	} else {
		log.Println("admin console disabled: ADMIN_ACCOUNTS / ADMIN_USERNAME / ADMIN_PASSWORD_HASH not set")
	}

	// 定时任务调度器：仅在 SCHEDULER_ENABLED=true 时启动，方便后续把 API 集群与定时任务集群拆分部署。
	if cfg.SchedulerEnabled {
		sch := scheduler.New()
		danmakuJob := jobs.NewDanmakuCleanup(companionRepo, cfg.DanmakuRetentionDays, cfg.DanmakuCleanupCron)
		companionAutoCloseJob := jobs.NewCompanionAutoClose(companionSvc)
		if err := sch.Register(danmakuJob, companionAutoCloseJob); err != nil {
			log.Printf("scheduler register failed, scheduler disabled: %v", err)
		} else {
			sch.Start()
			defer func() {
				stopCtx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
				defer cancel()
				if err := sch.Stop(stopCtx); err != nil {
					log.Printf("scheduler stop error: %v", err)
				}
			}()
			log.Printf("scheduler enabled: %d job(s)", len(sch.Jobs()))
		}
	} else {
		log.Println("scheduler disabled (SCHEDULER_ENABLED != true)")
	}

	h.Spin()
}

func setupLogging(logDir string) (*os.File, error) {
	if err := os.MkdirAll(logDir, 0o755); err != nil {
		return nil, err
	}

	logPath := filepath.Join(logDir, "server.log")
	file, err := os.OpenFile(logPath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
	if err != nil {
		return nil, err
	}

	writer := io.MultiWriter(os.Stdout, file)
	log.SetOutput(writer)
	hlog.SetOutput(writer)

	return file, nil
}
