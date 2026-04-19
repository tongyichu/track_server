package main

import (
	"context"
	"crypto/tls"
	"log"

	"github.com/cloudwego/hertz/pkg/app/server"

	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"

	"github.com/tongyichu/track_server/internal/config"
	"github.com/tongyichu/track_server/internal/handler"
	"github.com/tongyichu/track_server/internal/repository"
	"github.com/tongyichu/track_server/internal/service"
)

// main is the entrypoint of the Hertz HTTP server.
func main() {
	cfg := config.Load()

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
		}
	}

	trackSvc := service.NewTrackService(trackRepo, collectRepo)
	userSvc := service.NewUserService(userRepo)
	loginSvc := service.NewLoginService(userRepo, loginLogRepo, cfg.WechatAppID, cfg.WechatAppSecret)

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

	handler.RegisterRoutes(h, handler.Deps{TrackService: trackSvc, UserService: userSvc, LoginService: loginSvc})

	h.Spin()
}
