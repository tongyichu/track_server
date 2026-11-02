package service

import (
	"context"
	"github.com/tongyichu/track_server/internal/models"
	"github.com/tongyichu/track_server/internal/repository"
	"regexp"
	"strings"
	"time"
	"unicode/utf8"
)

type PermissionSnapshotService struct {
	repo repository.PermissionSnapshotRepository
}

func NewPermissionSnapshotService(repo repository.PermissionSnapshotRepository) *PermissionSnapshotService {
	return &PermissionSnapshotService{repo: repo}
}

var permissionInstallationID = regexp.MustCompile(`^[a-zA-Z0-9_-]{16,64}$`)

func (s *PermissionSnapshotService) Save(ctx context.Context, userID int64, snapshot *models.PermissionSnapshot) error {
	if userID <= 0 || !permissionInstallationID.MatchString(snapshot.InstallationID) || snapshot.Sequence <= 0 || snapshot.SchemaVersion <= 0 || snapshot.SchemaVersion > 100 || snapshot.VersionCode < 0 || snapshot.CollectedAt.IsZero() || len(snapshot.Items) > 256 || snapshot.Items == nil {
		return invalidArg("invalid permission snapshot")
	}
	for _, value := range []string{snapshot.Platform, snapshot.DeviceModel, snapshot.OSVersion, snapshot.AppVersion} {
		if utf8.RuneCountInString(value) > 128 {
			return invalidArg("device metadata too long")
		}
	}
	seen := map[string]bool{}
	for _, item := range snapshot.Items {
		if strings.TrimSpace(item.Key) == "" || len(item.Key) > 200 || len(item.Category) > 48 || len(item.State) > 48 || item.Category == "" || item.State == "" || seen[item.Key] {
			return invalidArg("invalid permission item")
		}
		seen[item.Key] = true
	}
	snapshot.UserID = userID
	snapshot.ReceivedAt = time.Now().UTC()
	return s.repo.Save(ctx, snapshot)
}
func (s *PermissionSnapshotService) List(ctx context.Context, userID int64) ([]models.PermissionSnapshot, error) {
	return s.repo.List(ctx, userID)
}
