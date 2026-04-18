package main

import (
	"context"
	"log"

	"github.com/cloudwego/hertz/pkg/app/server"

	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"

	"trackapp-server/internal/config"
	"trackapp-server/internal/handler"
	"trackapp-server/internal/repository"
	"trackapp-server/internal/service"
)

// main is the entrypoint of the Hertz HTTP server.
func main() {
	cfg := config.Load()

	ctx := context.Background()

	var trackRepo repository.TrackRepository
	var userRepo repository.UserRepository
	var collectRepo repository.CollectRepository

	if cfg.UseInMemory {
		trackRepo, userRepo, collectRepo = repository.NewInMemoryRepositories()
		log.Println("using in-memory repositories")
	} else {
		client, err := mongo.Connect(ctx, options.Client().ApplyURI(cfg.MongoURI))
		if err != nil {
			log.Printf("failed to connect mongo, fallback to memory: %v", err)
			trackRepo, userRepo, collectRepo = repository.NewInMemoryRepositories()
		} else {
			db := client.Database(cfg.MongoDBName)
			trackRepo = repository.NewMongoTrackRepository(db.Collection("tracks"))
			userRepo = repository.NewMongoUserRepository(db.Collection("users"))
			collectRepo = repository.NewMongoCollectRepository(db.Collection("track_collects"))
		}
	}

	trackSvc := service.NewTrackService(trackRepo, collectRepo)
	userSvc := service.NewUserService(userRepo)

	h := server.Default(server.WithHostPorts(cfg.ServerAddr))

	handler.RegisterRoutes(h, handler.Deps{TrackService: trackSvc, UserService: userSvc})

	log.Printf("server listening on %s", cfg.ServerAddr)
	h.Spin()
}
