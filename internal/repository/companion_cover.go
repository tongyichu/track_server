package repository

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"time"

	"github.com/tongyichu/track_server/internal/models"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
)

type CompanionCoverRepository interface {
	SaveCompanionCover(context.Context, string, int64, string, models.CompanionCoverPreview) error
}

var (
	_ CompanionCoverRepository = (*MySQLCompanionRepository)(nil)
	_ CompanionCoverRepository = (*MongoCompanionRepository)(nil)
	_ CompanionCoverRepository = (*InMemoryCompanionRepository)(nil)
)

func encodeCompanionCover(cover models.CompanionCoverPreview) string {
	data, _ := json.Marshal(cover)
	return string(data)
}

func ensureMySQLCompanionCoverColumn(ctx context.Context, db *sql.DB) error {
	var count int
	if err := db.QueryRowContext(ctx, `SELECT COUNT(*) FROM information_schema.COLUMNS WHERE TABLE_SCHEMA=DATABASE() AND TABLE_NAME='companion_sessions' AND COLUMN_NAME='cover_preview'`).Scan(&count); err != nil {
		return err
	}
	if count > 0 {
		return nil
	}
	_, err := db.ExecContext(ctx, `ALTER TABLE companion_sessions ADD COLUMN cover_preview JSON NULL COMMENT '同行静态预览四图及源版本' AFTER track_screenshot_url`)
	return err
}

func companionCoverWriteError(session *models.CompanionSession, owner int64) error {
	if session == nil {
		return ErrNotFound
	}
	if session.OwnerUserID != owner {
		return ErrForbidden
	}
	return ErrConflict
}

func (r *MySQLCompanionRepository) SaveCompanionCover(ctx context.Context, id string, owner int64, expected string, cover models.CompanionCoverPreview) error {
	result, err := r.db.ExecContext(ctx, `UPDATE companion_sessions SET cover_preview=?,updated_at=? WHERE session_id=? AND owner_user_id=? AND status='ended' AND (COALESCE(JSON_UNQUOTE(JSON_EXTRACT(cover_preview,'$.source_version')),'')=? OR JSON_UNQUOTE(JSON_EXTRACT(cover_preview,'$.source_version'))=?)`, encodeCompanionCover(cover), time.Now(), id, owner, expected, cover.SourceVersion)
	if err != nil {
		return err
	}
	n, err := result.RowsAffected()
	if err != nil || n > 0 {
		return err
	}
	session, err := r.FindSessionByID(ctx, id)
	if err != nil {
		return err
	}
	if session.OwnerUserID == owner && session.Status == models.CompanionSessionStatusEnded && session.CoverPreview == cover {
		return nil
	}
	return companionCoverWriteError(session, owner)
}

func (r *InMemoryCompanionRepository) SaveCompanionCover(_ context.Context, id string, owner int64, expected string, cover models.CompanionCoverPreview) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	session := r.sessions[id]
	if session == nil || session.OwnerUserID != owner || session.Status != models.CompanionSessionStatusEnded {
		return companionCoverWriteError(session, owner)
	}
	if session.CoverPreview.SourceVersion != expected && session.CoverPreview.SourceVersion != cover.SourceVersion {
		return ErrConflict
	}
	session.CoverPreview = cover
	session.UpdatedAt = time.Now()
	return nil
}

func (r *MongoCompanionRepository) SaveCompanionCover(ctx context.Context, id string, owner int64, expected string, cover models.CompanionCoverPreview) error {
	versions := bson.A{expected, cover.SourceVersion}
	if expected == "" {
		versions = append(versions, nil)
	}
	result, err := r.sessionCollection.UpdateOne(ctx, bson.M{"session_id": id, "owner_user_id": owner, "status": models.CompanionSessionStatusEnded,
		"cover_preview.source_version": bson.M{"$in": versions}}, bson.M{"$set": bson.M{"cover_preview": cover, "updated_at": time.Now()}})
	if err != nil {
		return err
	}
	if result.MatchedCount > 0 {
		return nil
	}
	var session models.CompanionSession
	err = r.sessionCollection.FindOne(ctx, bson.M{"session_id": id}).Decode(&session)
	if errors.Is(err, mongo.ErrNoDocuments) {
		return ErrNotFound
	}
	if err != nil {
		return err
	}
	return companionCoverWriteError(&session, owner)
}
