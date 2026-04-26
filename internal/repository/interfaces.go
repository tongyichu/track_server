package repository

import (
	"context"

	"errors"
	"fmt"
	"strconv"
	"strings"

	"github.com/tongyichu/track_server/internal/models"
)

const (
	// trackIDPrefix 是轨迹 ID 的固定业务前缀，便于与其他纯数字/纯编码 ID 区分。
	trackIDPrefix = "NO."
	// trackIDLength 是去掉前缀后的编码长度，固定为 8 位。
	trackIDLength = 8
	// trackIDBase 指定序列号转码时使用 36 进制，字符集为 0-9A-Z。
	trackIDBase = 36
	// trackIDSequenceLimit = 36^8，表示 8 位 base36 能容纳的序列总量上限。
	// 当序列值达到该阈值后，将无法再编码为固定 8 位，需要扩容编码长度或调整策略。
	trackIDSequenceLimit = 2821109907456 // 36^8
)

// Common repository errors.
var (
	// ErrNotFound indicates entity is not found in repository.
	ErrNotFound = errors.New("not found")
	// ErrAlreadyExists indicates entity creation conflicts with an existing unique key.
	ErrAlreadyExists = errors.New("already exists")
	// ErrForbidden indicates current user has no permission to operate the resource.
	ErrForbidden = errors.New("forbidden")
)

// TrackRepository defines persistence operations for Track entities.
type TrackRepository interface {
	// NextTrackID 返回一个新的轨迹 ID。
	// 该接口的职责是“分配”ID，而不是“写入”记录，便于不同存储实现各自选择最可靠的序列来源。
	// 当前 MySQL 通过独立自增序列表保证全局唯一；其他实现则提供等价的本地序列能力以兼容测试环境。
	NextTrackID(ctx context.Context) (string, error)
	Create(ctx context.Context, t *models.Track) error
	Update(ctx context.Context, t *models.Track) error
	FindByID(ctx context.Context, id string) (*models.Track, error)
	FindRunningByUserID(ctx context.Context, userID int64) (*models.Track, error)
	// ListByUserID 返回指定用户的轨迹列表（通常用于“我的轨迹”）。
	//
	// 约定：
	// - 仅返回未删除的轨迹；
	// - 列表默认按 start_time 倒序；
	// - 是否包含进行中的轨迹由具体实现决定（推荐只返回已结束轨迹）。
	ListByUserID(ctx context.Context, userID int64, cursor *models.TrackListCursor, limit int) ([]*models.Track, error)
	ListRecommend(ctx context.Context, userID int64, cursor *models.TrackListCursor, limit int) ([]*models.Track, error)
	Search(ctx context.Context, keyword string, cursor *models.TrackListCursor, limit int) ([]*models.Track, error)
}

// encodeTrackID 将全局递增序列编码成业务侧使用的轨迹 ID。
// 编码规则：
//  1. 输入必须是大于 0 的正整数序列；
//  2. 使用 base36 编码得到仅由 0-9A-Z 组成的字符串；
//  3. 不足 8 位时左侧补 0；
//  4. 最终统一加上固定前缀 "NO."。
//
// 例如：
//   - 1    -> NO.00000001
//   - 35   -> NO.0000000Z
//   - 36   -> NO.00000010
func encodeTrackID(sequence uint64) (string, error) {
	if sequence == 0 {
		return "", errors.New("track id sequence must be greater than 0")
	}
	if sequence >= trackIDSequenceLimit {
		return "", fmt.Errorf("track id sequence %d exceeds max encodable value", sequence)
	}

	encoded := strings.ToUpper(strconv.FormatUint(sequence, trackIDBase))
	if len(encoded) > trackIDLength {
		return "", fmt.Errorf("track id %q exceeds %d characters", encoded, trackIDLength)
	}
	if len(encoded) == trackIDLength {
		return trackIDPrefix + encoded, nil
	}
	return trackIDPrefix + strings.Repeat("0", trackIDLength-len(encoded)) + encoded, nil
}

// TrackWaypointRepository defines persistence operations for track multimedia waypoints.
type TrackWaypointRepository interface {
	Create(ctx context.Context, waypoint *models.TrackWaypoint) error
	ListByTrackID(ctx context.Context, trackID string) ([]*models.TrackWaypoint, error)
}

// UserRepository defines persistence operations for User entities.
type UserRepository interface {
	CreateIfNotExists(ctx context.Context, u *models.User) (*models.User, error)
	FindByID(ctx context.Context, id int64) (*models.User, error)
	FindByNickname(ctx context.Context, nickname string) (*models.User, error)
	Update(ctx context.Context, u *models.User) error
}

// CollectRepository defines persistence operations for track collections.
type CollectRepository interface {
	IsCollected(ctx context.Context, userID int64, trackID string) (bool, error)
	// ListByUserID lists collect records of a user in reverse chronological order.
	//
	// Order: created_at desc, track_id desc.
	// Cursor condition: (created_at, track_id) strictly less than cursor.
	ListByUserID(ctx context.Context, userID int64, cursor *models.TrackCollectCursor, limit int) ([]*models.TrackCollect, error)
	// RemoveByTrackID removes all collect records of the given track.
	// It is used to keep track_collects consistent when a track is deleted.
	RemoveByTrackID(ctx context.Context, trackID string) error
	// CountByTrackIDs 批量返回轨迹的收藏总数（track_id -> count）。
	//
	// 约定：
	// - 入参 trackIDs 允许重复/包含空串，实现应自行去重并忽略空串；
	// - 返回 map 中“未出现的 track_id”视为 0（调用方可直接用 map[key] 的零值）；
	// - 该统计与当前鉴权用户无关，仅表示全量收藏总数。
	CountByTrackIDs(ctx context.Context, trackIDs []string) (map[string]int64, error)
	AddCollect(ctx context.Context, userID int64, trackID string) error
	RemoveCollect(ctx context.Context, userID int64, trackID string) error
}

// NavigationRepository defines persistence operations for track navigation usage records.
//
// 设计说明：
// - “导航使用次数”按记录数统计（一次使用写一条记录），用于在列表接口展示轨迹被其他用户使用的次数。
// - 该统计与当前鉴权用户无关，只表示全量使用次数；服务层会在写入时避免记录“自己导航自己的轨迹”。
type NavigationRepository interface {
	// AddNavigation records one navigation usage for a track.
	AddNavigation(ctx context.Context, navigatorUserID int64, trackID string) error
	// CountByTrackIDs returns navigation usage count of each track.
	//
	// 约定与 CollectRepository.CountByTrackIDs 保持一致：
	// - 入参允许重复/包含空串，实现应自行去重并忽略空串；
	// - 返回 map 中未出现的 track_id 视为 0。
	CountByTrackIDs(ctx context.Context, trackIDs []string) (map[string]int64, error)
}

// LoginLogRepository defines persistence operations for login logs.
type LoginLogRepository interface {
	Create(ctx context.Context, log *models.LoginLog) error
	ListByUserID(ctx context.Context, userID int64, limit int) ([]*models.LoginLog, error)
}
