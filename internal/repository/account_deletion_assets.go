package repository

import (
	"context"
	"database/sql"
	"encoding/json"
	"sort"
)

type accountAssetSource struct {
	from, owner, values string
}

var accountAssetSources = []accountAssetSource{
	{"users u", "u.id", "JSON_ARRAY(u.avatar_url,u.background_url)"},
	{"track_records t", "t.user_id", "JSON_ARRAY(t.raw_track_url,t.track_screenshot_url,t.track_no_map_bg_screenshot_url)"},
	{"track_records t", "t.user_id", "t.preview_images"},
	{"track_waypoints w", "w.user_id", "w.media_urls"},
	{"track_submission_images i JOIN track_submissions s ON s.submission_id=i.submission_id", "s.user_id", "JSON_ARRAY(i.oss_url)"},
	{"track_comment_images i", "i.user_id", "JSON_ARRAY(i.oss_url,i.object_key)"},
	{"companion_sessions s", "s.owner_user_id", "JSON_ARRAY(s.track_screenshot_url)"},
	{"companion_sessions s", "s.owner_user_id", "s.cover_preview"},
}

func visitDeletionAssetJSON(payload []byte, visit func(string)) error {
	if len(payload) == 0 {
		return nil
	}
	var tree any
	if err := json.Unmarshal(payload, &tree); err != nil {
		return err
	}
	var visitValue func(any)
	visitValue = func(value any) {
		switch value := value.(type) {
		case string:
			visit(value)
		case []any:
			for _, item := range value {
				visitValue(item)
			}
		case map[string]any:
			for _, item := range value {
				visitValue(item)
			}
		}
	}
	visitValue(tree)
	return nil
}

func queryDeletionAssetJSON(ctx context.Context, tx *sql.Tx, query string, args []any, accept func([]byte) error) error {
	rows, err := tx.QueryContext(ctx, query, args...)
	if err != nil {
		return err
	}
	defer rows.Close()
	for rows.Next() {
		var payload []byte
		if err := rows.Scan(&payload); err != nil {
			return err
		}
		if err := accept(payload); err != nil {
			return err
		}
	}
	return rows.Err()
}

func accountDeletionAssets(ctx context.Context, tx *sql.Tx, userID int64, objectKey func(string) string) (AccountDeletionAssets, error) {
	var assets AccountDeletionAssets
	keys := map[string]struct{}{}
	for _, source := range accountAssetSources {
		query := "SELECT " + source.values + " FROM " + source.from + " WHERE " + source.owner + "=?"
		err := queryDeletionAssetJSON(ctx, tx, query, []any{userID}, func(payload []byte) error {
			return visitDeletionAssetJSON(payload, func(value string) {
				if key := objectKey(value); key != "" {
					keys[key] = struct{}{}
				}
			})
		})
		if err != nil {
			return assets, err
		}
	}
	for key := range keys {
		assets.Objects = append(assets.Objects, key)
	}
	sort.Strings(assets.Objects)
	if len(assets.Objects) > 0 {
		unshared, err := unsharedDeletionObjects(ctx, tx, userID, assets.Objects, objectKey)
		if err != nil {
			return assets, err
		}
		assets.Objects = unshared
	}
	rows, err := tx.QueryContext(ctx, `SELECT id,preview_images FROM track_records WHERE user_id=?`, userID)
	if err != nil {
		return assets, err
	}
	for rows.Next() {
		var item AccountDeletionTrackAssets
		var payload []byte
		if err := rows.Scan(&item.TrackID, &payload); err != nil {
			rows.Close()
			return assets, err
		}
		if len(payload) > 0 {
			if err := json.Unmarshal(payload, &item.Preview); err != nil {
				rows.Close()
				return assets, err
			}
		}
		assets.Tracks = append(assets.Tracks, item)
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return assets, err
	}
	if err := queryDeletionAssetJSON(ctx, tx, `SELECT JSON_ARRAY(CONCAT(i.submission_id,'_',i.image_id)) FROM track_submission_images i
		JOIN track_submissions s ON s.submission_id=i.submission_id WHERE s.user_id=?`, []any{userID}, func(payload []byte) error {
		return visitDeletionAssetJSON(payload, func(id string) { assets.SubmissionImageIDs = append(assets.SubmissionImageIDs, id) })
	}); err != nil {
		return assets, err
	}
	if err := queryDeletionAssetJSON(ctx, tx, `SELECT JSON_ARRAY(i.id) FROM track_comment_images i
		LEFT JOIN track_records t ON t.id=i.track_id WHERE i.user_id=? OR t.user_id=?`, []any{userID, userID}, func(payload []byte) error {
		return visitDeletionAssetJSON(payload, func(id string) { assets.CommentImageIDs = append(assets.CommentImageIDs, id) })
	}); err != nil {
		return assets, err
	}
	if err := queryDeletionAssetJSON(ctx, tx, `SELECT JSON_OBJECT('SessionID',session_id,'Version',
		COALESCE(JSON_UNQUOTE(JSON_EXTRACT(cover_preview,'$.source_version')),'')) FROM companion_sessions WHERE owner_user_id=?`, []any{userID}, func(payload []byte) error {
		var cover AccountDeletionCompanionCover
		if err := json.Unmarshal(payload, &cover); err != nil {
			return err
		}
		assets.CompanionCovers = append(assets.CompanionCovers, cover)
		return nil
	}); err != nil {
		return assets, err
	}
	return assets, nil
}

// UnsharedObjects checks other references after existing upload credentials have expired.
func (r *MySQLAccountDeletionStore) UnsharedObjects(ctx context.Context, userID int64, objects []string, objectKey func(string) string) ([]string, error) {
	tx, err := r.db.BeginTx(ctx, &sql.TxOptions{ReadOnly: true})
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()
	return unsharedDeletionObjects(ctx, tx, userID, objects, objectKey)
}

func unsharedDeletionObjects(ctx context.Context, tx *sql.Tx, userID int64, objects []string, objectKey func(string) string) ([]string, error) {
	keys := make(map[string]struct{}, len(objects))
	for _, key := range objects {
		keys[key] = struct{}{}
	}
	removeShared := func(payload []byte) error {
		return visitDeletionAssetJSON(payload, func(value string) { delete(keys, objectKey(value)) })
	}
	for _, source := range accountAssetSources {
		if len(keys) == 0 {
			return nil, nil
		}
		query := "SELECT " + source.values + " FROM " + source.from + " WHERE " + source.owner + "<>?"
		if err := queryDeletionAssetJSON(ctx, tx, query, []any{userID}, removeShared); err != nil {
			return nil, err
		}
	}
	for _, query := range []string{`SELECT data_json FROM scenic_spots`, `SELECT payload FROM region_catalog_state`} {
		if len(keys) == 0 {
			return nil, nil
		}
		if err := queryDeletionAssetJSON(ctx, tx, query, nil, removeShared); err != nil {
			return nil, err
		}
	}
	remaining := make([]string, 0, len(keys))
	for _, key := range objects {
		if _, ok := keys[key]; ok {
			remaining = append(remaining, key)
		}
	}
	return remaining, nil
}
