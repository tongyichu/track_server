package main

import (
	"context"
	"crypto/tls"
	"io"
	"log"
	"os"
	"path/filepath"

	"github.com/cloudwego/hertz/pkg/app/server"
	"github.com/cloudwego/hertz/pkg/common/hlog"

	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"

	"github.com/tongyichu/track_server/internal/config"
	"github.com/tongyichu/track_server/internal/handler"
	"github.com/tongyichu/track_server/internal/middleware"
	"github.com/tongyichu/track_server/internal/repository"
	"github.com/tongyichu/track_server/internal/service"
)

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

	if cfg.UseInMemory {
		trackRepo, userRepo, collectRepo, loginLogRepo = repository.NewInMemoryRepositories()
		log.Println("using in-memory repositories")
	} else if cfg.UseMySQL {
		db, err := repository.OpenMySQL(cfg.MySQLDSN)
		if err != nil {
			log.Printf("failed to open mysql, fallback to memory: %v", err)
			trackRepo, userRepo, collectRepo, loginLogRepo = repository.NewInMemoryRepositories()
		} else if err := db.PingContext(ctx); err != nil {
			log.Printf("failed to ping mysql, fallback to memory: %v", err)
			_ = db.Close()
			trackRepo, userRepo, collectRepo, loginLogRepo = repository.NewInMemoryRepositories()
		} else {
			tr, ur, cr, lr, err := repository.NewMySQLRepositories(ctx, db)
			if err != nil {
				log.Printf("failed to init mysql schema, fallback to memory: %v", err)
				_ = db.Close()
				trackRepo, userRepo, collectRepo, loginLogRepo = repository.NewInMemoryRepositories()
			} else {
				defer func() {
					_ = db.Close()
				}()
				trackRepo, userRepo, collectRepo, loginLogRepo = tr, ur, cr, lr
				log.Println("using mysql repositories")
			}
		}
	} else {
		log.Println("using mongo repositories")
		client, err := mongo.Connect(ctx, options.Client().ApplyURI(cfg.MongoURI))
		if err != nil {
			log.Printf("failed to connect mongo, fallback to memory: %v", err)
			trackRepo, userRepo, collectRepo, loginLogRepo = repository.NewInMemoryRepositories()
		} else {
			db := client.Database(cfg.MongoDBName)
			trackRepo = repository.NewMongoTrackRepository(db.Collection("tracks"))
			userRepo = repository.NewMongoUserRepository(db.Collection("users"))
			collectRepo = repository.NewMongoCollectRepository(db.Collection("track_collects"))
			loginLogRepo = repository.NewMongoLoginLogRepository(db.Collection("login_log"))
			log.Println("mongo repositories initialized")
		}
	}

	trackSvc := service.NewTrackService(trackRepo, collectRepo)
	userSvc := service.NewUserService(userRepo)
	loginSvc := service.NewLoginService(userRepo, loginLogRepo, cfg.WechatAppID, cfg.WechatAppSecret, cfg.JWTSecret)

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
		)
		log.Printf("server listening on %s (HTTPS)", cfg.ServerAddr)
	} else {
		h = server.Default(server.WithHostPorts(cfg.ServerAddr))
		log.Printf("server listening on %s (HTTP)", cfg.ServerAddr)
	}

	tokenBlacklist := middleware.NewTokenBlacklist()
	defer tokenBlacklist.Close()

	handler.RegisterRoutes(h, handler.Deps{
		TrackService:   trackSvc,
		UserService:    userSvc,
		LoginService:   loginSvc,
		JWTSecret:      cfg.JWTSecret,
		TokenBlacklist: tokenBlacklist,
	})

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
