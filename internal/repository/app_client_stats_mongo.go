package repository

import (
	"context"
	"errors"

	"github.com/tongyichu/track_server/internal/models"
)

// MongoAppClientStatsRepository is a stub of AppClientStatsRepository backed by MongoDB.
// 与 MongoAppReleaseRepository 一致：Mongo 部署暂不实现发布与客户端统计，返回未实现错误。
type MongoAppClientStatsRepository struct{}

// NewMongoAppClientStatsRepository constructs the stub repository.
func NewMongoAppClientStatsRepository() *MongoAppClientStatsRepository {
	return &MongoAppClientStatsRepository{}
}

// TouchClientDevice is not implemented for MongoDB.
func (r *MongoAppClientStatsRepository) TouchClientDevice(context.Context, *models.AppClientDevice) error {
	return errors.New("MongoAppClientStatsRepository.TouchClientDevice not implemented")
}

// GetClientStatistics is not implemented for MongoDB.
func (r *MongoAppClientStatsRepository) GetClientStatistics(context.Context, int) (*models.AppClientStatistics, error) {
	return nil, errors.New("MongoAppClientStatsRepository.GetClientStatistics not implemented")
}
