package main

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"log"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"github.com/go-sql-driver/mysql"
	"github.com/tongyichu/track_server/internal/config"
	"github.com/tongyichu/track_server/internal/models"
	"github.com/tongyichu/track_server/internal/repository"
	"github.com/tongyichu/track_server/internal/service"
)

type titlePlan struct {
	Version  int                        `json:"version"`
	Database string                     `json:"database_fingerprint"`
	Changes  []service.TrackTitleChange `json:"changes"`
}

type options struct {
	planPath, applyPath, resultPath, rawDir string
	userID                                  int64
	maxChanges                              int
	downloadRaw                             bool
	includeDatedTitles                      bool
}

func main() {
	var opt options
	flag.StringVar(&opt.planPath, "plan", "", "Write a preview JSON plan; does not update database records")
	flag.StringVar(&opt.applyPath, "apply-plan", "", "Apply exactly the changes in an existing plan")
	flag.StringVar(&opt.resultPath, "result", "", "New JSONL result file, required with -apply-plan")
	flag.StringVar(&opt.rawDir, "raw-track-dir", "", "Existing raw-track cache directory; defaults to LOG_DIR/static/raw_tracks")
	flag.Int64Var(&opt.userID, "user-id", 0, "Limit preview to this owner; 0 scans all owners")
	flag.IntVar(&opt.maxChanges, "max-changes", 0, "Maximum preview candidates; 0 means no limit")
	flag.BoolVar(&opt.downloadRaw, "download-raw", false, "Allow missing raw tracks to be downloaded through configured internal OSS")
	flag.BoolVar(&opt.includeDatedTitles, "include-dated-titles", false, "Also clean date prefixes/suffixes and previous date + workout fallback titles")
	flag.Parse()
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()
	if err := run(ctx, opt); err != nil {
		log.Fatal(err)
	}
}

func run(ctx context.Context, opt options) error {
	if (opt.planPath == "") == (opt.applyPath == "") || opt.userID < 0 || opt.maxChanges < 0 {
		return errors.New("choose exactly one of -plan or -apply-plan; filters must be nonnegative")
	}
	if opt.applyPath != "" && opt.resultPath == "" {
		return errors.New("-apply-plan requires a new -result file")
	}
	if opt.applyPath != "" && (opt.userID != 0 || opt.maxChanges != 0 || opt.downloadRaw || opt.rawDir != "" || opt.includeDatedTitles) {
		return errors.New("preview filters cannot be used with -apply-plan")
	}
	cfg := config.Load()
	if cfg.MySQLDSN == "" || cfg.UseInMemory {
		return errors.New("a real MYSQL_DSN is required; backfill never falls back to in-memory storage")
	}
	dsn, err := mysql.ParseDSN(cfg.MySQLDSN)
	if err != nil {
		return errors.New("invalid MYSQL_DSN")
	}
	fingerprint := fmt.Sprintf("%x", sha256.Sum256([]byte(dsn.Net+"|"+dsn.Addr+"|"+dsn.DBName)))
	db, err := repository.OpenMySQL(cfg.MySQLDSN)
	if err != nil {
		return errors.New("cannot open MySQL connection")
	}
	defer db.Close()
	connectCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
	err = db.PingContext(connectCtx)
	cancel()
	if err != nil {
		return fmt.Errorf("cannot connect to MySQL: %w", err)
	}
	if opt.applyPath != "" {
		return applyPlan(ctx, db, fingerprint, opt.applyPath, opt.resultPath)
	}
	if opt.rawDir == "" {
		opt.rawDir = filepath.Join(cfg.LogDir, "static", "raw_tracks")
	}
	cache, err := service.NewAssetCacheService(opt.rawDir, "/api/v1/static/raw_tracks",
		[]string{".dat", ".json", ".gpx", ".kmz", ".zip", ".kml"}, ".dat")
	if err != nil {
		return err
	}
	if opt.downloadRaw {
		if cfg.OSSInternalEndpoint == "" {
			return errors.New("-download-raw requires OSS_INTERNAL_ENDPOINT; public endpoint fallback is forbidden")
		}
		downloader, err := service.NewOSSTokenService(cfg.AliyunSTSRegion, cfg.AliyunAccessKeyID,
			cfg.AliyunAccessKeySecret, cfg.AliyunRoleARN, cfg.AliyunSTSDurationSec, cfg.AliyunRoleSessionPref,
			cfg.OSSBucket, cfg.OSSRegion, cfg.OSSEndpoint, cfg.OSSInternalEndpoint, cfg.OSSUploadPrefix)
		if err != nil {
			return err
		}
		cache.SetDownloader(downloader)
	}
	planner := service.TrackTitleBackfillPlanner{
		Submissions: repository.NewMySQLTrackSubmissionRepository(db), RawTracks: cache,
		IncludeDatedTitles: opt.includeDatedTitles,
	}
	tracks := repository.NewMySQLTrackRepository(db)
	plan := titlePlan{Version: 1, Database: fingerprint, Changes: []service.TrackTitleChange{}}
	var cursor *models.TrackListCursor
	scanned := 0
	for {
		page, err := tracks.ListAll(ctx, cursor, 200)
		if err != nil {
			return err
		}
		if len(page) == 0 {
			break
		}
		for _, track := range page {
			scanned++
			if opt.userID != 0 && opt.userID != track.UserID {
				continue
			}
			change, err := planner.Plan(ctx, track)
			if err != nil {
				return fmt.Errorf("plan %s: %w", track.ID, err)
			}
			if change != nil {
				plan.Changes = append(plan.Changes, *change)
			}
			if opt.maxChanges > 0 && len(plan.Changes) >= opt.maxChanges {
				break
			}
		}
		if opt.maxChanges > 0 && len(plan.Changes) >= opt.maxChanges {
			break
		}
		last := page[len(page)-1]
		cursor = &models.TrackListCursor{StartTime: last.StartTime, ID: last.ID}
	}
	file, err := createOutput(opt.planPath)
	if err != nil {
		return err
	}
	defer file.Close()
	encoder := json.NewEncoder(file)
	encoder.SetIndent("", "  ")
	if err := encoder.Encode(plan); err != nil {
		return err
	}
	if err := file.Sync(); err != nil {
		return err
	}
	log.Printf("preview complete: scanned=%d candidates=%d plan=%s; database unchanged", scanned, len(plan.Changes), opt.planPath)
	return nil
}

func createOutput(path string) (*os.File, error) {
	return os.OpenFile(path, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o600)
}

func readPlan(path, fingerprint string) (*titlePlan, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer file.Close()
	decoder := json.NewDecoder(io.LimitReader(file, 64*1024*1024+1))
	decoder.DisallowUnknownFields()
	var plan titlePlan
	if err := decoder.Decode(&plan); err != nil {
		return nil, err
	}
	var extra interface{}
	if err := decoder.Decode(&extra); err != io.EOF {
		return nil, errors.New("plan has trailing data or exceeds size limit")
	}
	if plan.Version != 1 || plan.Database != fingerprint {
		return nil, errors.New("plan version or target database does not match")
	}
	seen := make(map[string]bool, len(plan.Changes))
	for _, change := range plan.Changes {
		if err := change.Validate(); err != nil {
			return nil, fmt.Errorf("invalid change %s: %w", change.TrackID, err)
		}
		if seen[change.TrackID] {
			return nil, fmt.Errorf("duplicate track %s", change.TrackID)
		}
		seen[change.TrackID] = true
	}
	return &plan, nil
}

// Only the title is modified, and concurrent edits, deletion or recording restart win.
const replaceTitleSQL = `UPDATE track_records SET title=?, updated_at=?
	WHERE id=? AND user_id=? AND BINARY title=BINARY ? AND status<>0 AND is_running=0`

func applyPlan(ctx context.Context, db *sql.DB, fingerprint, path, resultPath string) error {
	plan, err := readPlan(path, fingerprint)
	if err != nil {
		return err
	}
	file, err := createOutput(resultPath)
	if err != nil {
		return err
	}
	defer file.Close()
	encoder := json.NewEncoder(file)
	updated, skipped := 0, 0
	for _, change := range plan.Changes {
		result, err := db.ExecContext(ctx, replaceTitleSQL, change.NewTitle, time.Now(),
			change.TrackID, change.UserID, change.OldTitle)
		if err != nil {
			return fmt.Errorf("update %s failed after %d updates: %w", change.TrackID, updated, err)
		}
		rows, err := result.RowsAffected()
		if err != nil {
			return err
		}
		status := "skipped"
		if rows == 1 {
			status = "updated"
			updated++
		} else {
			skipped++
		}
		if err := encoder.Encode(struct {
			service.TrackTitleChange
			Status string `json:"status"`
		}{change, status}); err != nil {
			return err
		}
		if err := file.Sync(); err != nil {
			return err
		}
	}
	log.Printf("apply complete: updated=%d skipped=%d result=%s", updated, skipped, strings.TrimSpace(resultPath))
	return nil
}
