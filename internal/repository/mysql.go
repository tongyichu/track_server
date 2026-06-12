package repository

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/go-sql-driver/mysql"

	"github.com/tongyichu/track_server/internal/models"
)

// NewMySQLRepositories wires MySQL-backed repositories and ensures schema exists.
func NewMySQLRepositories(ctx context.Context, db *sql.DB) (TrackRepository, UserRepository, CollectRepository, LoginLogRepository, NavigationRepository, AppReleaseRepository, CompanionRepository, error) {
	if err := ensureMySQLSchema(ctx, db); err != nil {
		return nil, nil, nil, nil, nil, nil, nil, err
	}
	return NewMySQLTrackRepository(db), NewMySQLUserRepository(db), NewMySQLCollectRepository(db), NewMySQLLoginLogRepository(db), NewMySQLNavigationRepository(db), NewMySQLAppReleaseRepository(db), NewMySQLCompanionRepository(db), nil
}

func ensureMySQLSchema(ctx context.Context, db *sql.DB) error {
	stmts := []string{
		`CREATE TABLE IF NOT EXISTS users (
			id BIGINT PRIMARY KEY,
			nickname VARCHAR(255) NOT NULL DEFAULT '',
			avatar_url VARCHAR(255) COMMENT '用户头像URI',
			signature TEXT,
			phone VARCHAR(32) NOT NULL DEFAULT '',
			client_language VARCHAR(64) NOT NULL DEFAULT '',
			token_version BIGINT NOT NULL DEFAULT 1,
			created_at DATETIME(6) NOT NULL,
			updated_at DATETIME(6) NOT NULL,
			INDEX idx_users_updated_at (updated_at)
		) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;`,
		`CREATE TABLE IF NOT EXISTS track_records (
			id VARCHAR(64) NOT NULL,
			user_id BIGINT UNSIGNED NOT NULL,
			session_id VARCHAR(64) NOT NULL DEFAULT '' COMMENT '关联的同行会话ID',
			city_code VARCHAR(16) NOT NULL DEFAULT '' COMMENT '城市Code',
			locate_addr VARCHAR(255) NOT NULL DEFAULT '' COMMENT '轨迹的具体位置信息',
			track_type VARCHAR(32) NOT NULL DEFAULT '' COMMENT '轨迹类型',
			source_tag VARCHAR(64) NOT NULL DEFAULT '' COMMENT '轨迹来源/运营标签',
			coordinate_system VARCHAR(32) NOT NULL DEFAULT '' COMMENT '坐标系',
			title VARCHAR(128) NOT NULL DEFAULT '',
			start_time DATETIME NOT NULL,
			end_time DATETIME,
			distance DECIMAL(10,2) DEFAULT 0.00,
			duration INT UNSIGNED DEFAULT 0,
			calories_burned DECIMAL(10,2) DEFAULT 0.00 COMMENT '热量消耗(千卡)',
			elevation_gain INT DEFAULT 0,
			raw_track_url VARCHAR(255),
			track_screenshot_url VARCHAR(255),
			track_no_map_bg_screenshot_url VARCHAR(255) COMMENT '没有地图背景的轨迹路线截图URI',
			is_running TINYINT(1) NOT NULL DEFAULT 1,
			status TINYINT NOT NULL DEFAULT 1,
			avg_speed_kmh DOUBLE NOT NULL DEFAULT 1,
			created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
			updated_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
			deleted_at DATETIME NULL COMMENT '删除时间',
			PRIMARY KEY (id),
			KEY idx_track_session (session_id),
			KEY idx_track_source_tag (source_tag),
			KEY idx_user_running (user_id, is_running, start_time),
			KEY idx_user_time (user_id, start_time)
		) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;`,
		// `track_id_sequences` 是轨迹 ID 的全局发号表。
		// 每插入一行就能拿到一个新的自增 id，再由业务层编码成 `NO.` + 8 位 base36 的轨迹 ID。
		// 表结构保持极简，只保留自增主键，减少额外约束对发号性能和稳定性的影响。
		`CREATE TABLE IF NOT EXISTS track_id_sequences (
			id BIGINT UNSIGNED NOT NULL AUTO_INCREMENT,
			PRIMARY KEY (id)
		) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;`,
		`CREATE TABLE IF NOT EXISTS track_waypoints (
			id BIGINT UNSIGNED NOT NULL AUTO_INCREMENT,
			track_id VARCHAR(128) NOT NULL,
			user_id BIGINT UNSIGNED NOT NULL,
			lat DECIMAL(10,7) NOT NULL,
			lng DECIMAL(10,7) NOT NULL,
			elevation INT NOT NULL DEFAULT 0,
			node_time DATETIME NOT NULL,
			media_type TINYINT NOT NULL,
			content TEXT,
			media_urls JSON,
			created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
			PRIMARY KEY (id),
			KEY idx_track_id (track_id)
		) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;`,
		`CREATE TABLE IF NOT EXISTS track_collects (
			user_id BIGINT NOT NULL,
			track_id VARCHAR(64) NOT NULL,
			created_at DATETIME(6) NOT NULL,
			PRIMARY KEY (user_id, track_id),
			INDEX idx_collects_track (track_id)
		) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;`,
		`CREATE TABLE IF NOT EXISTS user_follows (
			follower_user_id BIGINT NOT NULL COMMENT '关注者用户ID',
			followee_user_id BIGINT NOT NULL COMMENT '被关注者用户ID',
			created_at DATETIME(6) NOT NULL DEFAULT CURRENT_TIMESTAMP(6) COMMENT '关注时间',
			PRIMARY KEY (follower_user_id, followee_user_id),
			KEY idx_user_follows_followee (followee_user_id, created_at, follower_user_id),
			KEY idx_user_follows_follower (follower_user_id, created_at, followee_user_id)
		) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COMMENT='用户关注关系表';`,
		`CREATE TABLE IF NOT EXISTS track_navigations (
			id BIGINT UNSIGNED NOT NULL AUTO_INCREMENT,
			track_id VARCHAR(64) NOT NULL COMMENT '轨迹ID',
			navigator_user_id BIGINT NOT NULL COMMENT '导航使用者用户ID',
			created_at DATETIME(6) NOT NULL DEFAULT CURRENT_TIMESTAMP(6) COMMENT '使用时间',
			PRIMARY KEY (id),
			INDEX idx_nav_track (track_id),
			INDEX idx_nav_user (navigator_user_id)
		) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COMMENT='轨迹导航使用记录表';`,
		`CREATE TABLE IF NOT EXISTS user_achievement_rewards (
			id BIGINT UNSIGNED NOT NULL AUTO_INCREMENT,
			user_id BIGINT NOT NULL,
			reward_code VARCHAR(64) NOT NULL,
			source_track_id VARCHAR(64) NOT NULL DEFAULT '',
			source_session_id VARCHAR(64) NOT NULL DEFAULT '',
			progress_snapshot TEXT,
			earned_at DATETIME(6) NOT NULL,
			PRIMARY KEY (id),
			UNIQUE KEY uk_user_reward (user_id, reward_code),
			KEY idx_user_achievement_earned (user_id, earned_at, id)
		) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COMMENT='用户成就奖励记录表';`,
		`CREATE TABLE IF NOT EXISTS login_log (
			id BIGINT PRIMARY KEY AUTO_INCREMENT,
			user_id BIGINT NOT NULL,
			login_type VARCHAR(20) NOT NULL COMMENT 'sms/wechat/apple',
			ip VARCHAR(45),
			device_id VARCHAR(128) COMMENT '设备唯一标识（客户端上报），用于判断是否新设备登录、设备维度限流和风控',
			platform VARCHAR(10) COMMENT '客户端平台：ios / android，用于按平台统计和排查问题',
			created_at DATETIME DEFAULT CURRENT_TIMESTAMP
		) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;`,
		`CREATE TABLE IF NOT EXISTS companion_sessions (
				session_id VARCHAR(64) NOT NULL,
				owner_user_id BIGINT NOT NULL,
				status VARCHAR(16) NOT NULL,
				visibility VARCHAR(16) NOT NULL DEFAULT 'private',
				join_token VARCHAR(128) NOT NULL,
				title VARCHAR(64) NOT NULL DEFAULT '',
				track_type VARCHAR(32) NOT NULL DEFAULT '',
				locate_addr VARCHAR(255) NOT NULL DEFAULT '',
				max_members INT NOT NULL DEFAULT 8,
				danmaku_enabled TINYINT(1) NOT NULL DEFAULT 1,
				total_distance DOUBLE NOT NULL DEFAULT 0 COMMENT '同行总里程，单位米',
				total_duration BIGINT NOT NULL DEFAULT 0 COMMENT '同行总耗时，单位秒',
				track_screenshot_url VARCHAR(255) NOT NULL DEFAULT '' COMMENT '同行轨迹截图文件在对象存储中的地址',
				actual_participant_count BIGINT NOT NULL DEFAULT 0 COMMENT '实际参与同行人数',
				started_at DATETIME(6) NOT NULL,
				ended_at DATETIME(6) NULL,
				end_reason VARCHAR(32) NOT NULL DEFAULT '',
				end_source VARCHAR(32) NOT NULL DEFAULT '',
				end_operator_user_id BIGINT NOT NULL DEFAULT 0,
				created_at DATETIME(6) NOT NULL,
				updated_at DATETIME(6) NOT NULL,
			PRIMARY KEY (session_id),
			UNIQUE KEY uk_companion_join_token (join_token),
			KEY idx_companion_owner_status (owner_user_id, status),
			KEY idx_companion_session_status_ended (status, ended_at)
		) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COMMENT='同行会话表';`,
		`CREATE TABLE IF NOT EXISTS companion_session_members (
			session_id VARCHAR(64) NOT NULL,
			user_id BIGINT NOT NULL,
			role VARCHAR(16) NOT NULL,
			member_status VARCHAR(16) NOT NULL,
			presence_status VARCHAR(16) NOT NULL,
			joined_at DATETIME(6) NOT NULL,
			left_at DATETIME(6) NULL,
			last_seen_at DATETIME(6) NULL,
			mqtt_client_id VARCHAR(128) NOT NULL DEFAULT '',
			mqtt_principal VARCHAR(128) NOT NULL DEFAULT '',
			PRIMARY KEY (session_id, user_id),
			KEY idx_companion_member_status (session_id, member_status),
			KEY idx_companion_presence (session_id, presence_status)
		) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COMMENT='同行会话成员表';`,
		`CREATE TABLE IF NOT EXISTS companion_live_positions (
			session_id VARCHAR(64) NOT NULL,
			user_id BIGINT NOT NULL,
			track_id VARCHAR(64) NULL,
			latitude DOUBLE NOT NULL,
			longitude DOUBLE NOT NULL,
			coordinate_system VARCHAR(16) NOT NULL,
			speed_kmh DOUBLE NOT NULL DEFAULT 0,
			heading DOUBLE NOT NULL DEFAULT 0,
			accuracy_m DOUBLE NOT NULL DEFAULT 0,
			altitude DOUBLE NOT NULL DEFAULT 0,
			recorded_at DATETIME(6) NOT NULL,
			seq BIGINT NOT NULL DEFAULT 0,
			source VARCHAR(16) NOT NULL DEFAULT 'http',
			created_at DATETIME(6) NOT NULL,
			updated_at DATETIME(6) NOT NULL,
			PRIMARY KEY (session_id, user_id),
			KEY idx_companion_positions_recorded (session_id, recorded_at)
		) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COMMENT='同行会话最新位置快照表';`,
		`CREATE TABLE IF NOT EXISTS companion_danmakus (
			id BIGINT UNSIGNED NOT NULL AUTO_INCREMENT,
			session_id VARCHAR(64) NOT NULL,
			user_id BIGINT NOT NULL,
			content VARCHAR(200) NOT NULL,
			created_at DATETIME(6) NOT NULL,
			PRIMARY KEY (id),
			KEY idx_companion_danmaku_session_time (session_id, created_at),
			KEY idx_companion_danmaku_session_user_time (session_id, user_id, created_at)
		) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COMMENT='同行文字弹幕表';`,
		`CREATE TABLE IF NOT EXISTS companion_events (
			id BIGINT UNSIGNED NOT NULL AUTO_INCREMENT,
			session_id VARCHAR(64) NOT NULL COMMENT '同行会话ID',
			owner_user_id BIGINT NOT NULL COMMENT '房主用户ID',
			event_type VARCHAR(32) NOT NULL COMMENT '事件类型',
			target_user_id BIGINT NOT NULL DEFAULT 0 COMMENT '事件关联成员用户ID；无关联成员为0',
			title VARCHAR(64) NOT NULL DEFAULT '' COMMENT '事件标题',
			content VARCHAR(500) NOT NULL DEFAULT '' COMMENT '事件内容',
			event_time DATETIME(6) NOT NULL COMMENT '事件发生时间',
			client_event_id VARCHAR(128) NOT NULL COMMENT '客户端幂等事件ID',
			metadata_json TEXT COMMENT '客户端扩展JSON对象',
			created_at DATETIME(6) NOT NULL COMMENT '入库时间',
			PRIMARY KEY (id),
			UNIQUE KEY uk_companion_event_client (session_id, client_event_id),
			KEY idx_companion_events_session_time (session_id, event_time, id)
		) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COMMENT='同行关键事件表';`,
		`CREATE TABLE IF NOT EXISTS app_releases (
			id BIGINT UNSIGNED NOT NULL AUTO_INCREMENT,
			platform VARCHAR(16) NOT NULL COMMENT 'android / ios',
			version_name VARCHAR(32) NOT NULL,
			version_code INT UNSIGNED NOT NULL,
			min_supported_version_code INT UNSIGNED NOT NULL DEFAULT 0,
			package_url VARCHAR(512) NOT NULL DEFAULT '',
			package_size BIGINT UNSIGNED NOT NULL DEFAULT 0,
			package_md5 VARCHAR(64) NOT NULL DEFAULT '',
			release_notes TEXT,
			force_update TINYINT NOT NULL DEFAULT 0,
			status VARCHAR(16) NOT NULL DEFAULT 'published',
			operator_user_id BIGINT NOT NULL DEFAULT 0,
			operator_name VARCHAR(64) NOT NULL DEFAULT '',
			created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
			updated_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
			PRIMARY KEY (id),
			UNIQUE KEY uk_platform_version_code (platform, version_code),
			KEY idx_platform_status_code (platform, status, version_code)
		) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COMMENT='App 发布信息表';`,
		`CREATE TABLE IF NOT EXISTS user_feedbacks (
			id BIGINT UNSIGNED NOT NULL AUTO_INCREMENT,
			feedback_id VARCHAR(32) NOT NULL,
			user_id BIGINT NOT NULL,
			content TEXT NOT NULL,
			images_json JSON NULL,
			contact VARCHAR(128) NOT NULL DEFAULT '',
			app_version VARCHAR(32) NOT NULL DEFAULT '',
			platform VARCHAR(32) NOT NULL DEFAULT '',
			device_model VARCHAR(128) NOT NULL DEFAULT '',
			system_version VARCHAR(64) NOT NULL DEFAULT '',
			status VARCHAR(32) NOT NULL DEFAULT 'pending',
			reply TEXT NULL,
			created_at DATETIME(6) NOT NULL,
			updated_at DATETIME(6) NOT NULL,
			PRIMARY KEY (id),
			UNIQUE KEY uk_feedback_id (feedback_id),
			KEY idx_feedback_user_created (user_id, created_at, feedback_id),
			KEY idx_feedback_status_created (status, created_at, feedback_id)
		) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COMMENT='用户意见反馈表';`,
		`CREATE TABLE IF NOT EXISTS analytics_sync_summaries (
			id BIGINT UNSIGNED NOT NULL AUTO_INCREMENT,
			job_name VARCHAR(64) NOT NULL DEFAULT 'analytics_sync' COMMENT '同步任务名',
			status VARCHAR(16) NOT NULL COMMENT 'success / partial / failed',
			started_at DATETIME(6) NOT NULL COMMENT '任务开始时间',
			ended_at DATETIME(6) NOT NULL COMMENT '任务结束时间',
			duration_ms BIGINT NOT NULL DEFAULT 0 COMMENT '任务耗时毫秒',
			scanned_files INT NOT NULL DEFAULT 0 COMMENT '扫描到的本地文件数',
			uploaded_files INT NOT NULL DEFAULT 0 COMMENT '成功上传文件数',
			failed_files INT NOT NULL DEFAULT 0 COMMENT '失败文件数',
			total_bytes BIGINT NOT NULL DEFAULT 0 COMMENT '成功上传的数据量字节数',
			oss_prefix VARCHAR(255) NOT NULL DEFAULT '' COMMENT 'OSS ODS 归档前缀',
			files_json JSON NULL COMMENT '文件级同步摘要',
			error_message TEXT NULL COMMENT '任务级错误摘要',
			created_at DATETIME(6) NOT NULL,
			PRIMARY KEY (id),
			KEY idx_analytics_sync_started (started_at),
			KEY idx_analytics_sync_status_started (status, started_at)
		) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COMMENT='埋点文件同步摘要表';`,
	}

	for _, stmt := range stmts {
		if _, err := db.ExecContext(ctx, stmt); err != nil {
			return err
		}
	}
	if err := ensureMySQLUsersPhoneColumn(ctx, db); err != nil {
		return err
	}
	if err := ensureMySQLUsersTokenVersionColumn(ctx, db); err != nil {
		return err
	}
	if err := ensureMySQLTrackScreenshotURLColumn(ctx, db); err != nil {
		return err
	}
	if err := ensureMySQLTrackNoMapBgScreenshotURLColumn(ctx, db); err != nil {
		return err
	}
	if err := ensureMySQLTrackIsRunningColumn(ctx, db); err != nil {
		return err
	}
	if err := ensureMySQLTrackAvgSpeedKmhColumn(ctx, db); err != nil {
		return err
	}
	if err := ensureMySQLTrackCaloriesBurnedColumn(ctx, db); err != nil {
		return err
	}
	if err := ensureMySQLTrackCityCodeColumn(ctx, db); err != nil {
		return err
	}
	if err := ensureMySQLTrackSessionIDColumn(ctx, db); err != nil {
		return err
	}
	if err := ensureMySQLTrackSessionIDIndex(ctx, db); err != nil {
		return err
	}
	if err := ensureMySQLTrackLocateAddrColumn(ctx, db); err != nil {
		return err
	}
	if err := ensureMySQLTrackDeletedAtColumn(ctx, db); err != nil {
		return err
	}
	if err := ensureMySQLTrackTypeColumn(ctx, db); err != nil {
		return err
	}
	if err := ensureMySQLTrackSourceTagColumn(ctx, db); err != nil {
		return err
	}
	if err := ensureMySQLTrackSourceTagIndex(ctx, db); err != nil {
		return err
	}
	if err := ensureMySQLTrackCoordinateSystemColumn(ctx, db); err != nil {
		return err
	}
	if err := ensureMySQLUserIDColumnsBigint(ctx, db); err != nil {
		return err
	}
	if err := ensureMySQLTrackWaypointTrackIDColumn(ctx, db); err != nil {
		return err
	}
	if err := ensureMySQLCompanionSessionTrackTypeColumn(ctx, db); err != nil {
		return err
	}
	if err := ensureMySQLCompanionSessionLocateAddrColumn(ctx, db); err != nil {
		return err
	}
	if err := ensureMySQLCompanionSessionVisibilityColumn(ctx, db); err != nil {
		return err
	}
	if err := ensureMySQLCompanionSessionDanmakuEnabledColumn(ctx, db); err != nil {
		return err
	}
	if err := ensureMySQLCompanionSessionStatsColumns(ctx, db); err != nil {
		return err
	}
	if err := ensureMySQLCompanionSessionEndAuditColumns(ctx, db); err != nil {
		return err
	}
	if err := ensureMySQLCompanionSessionStatusEndedIndex(ctx, db); err != nil {
		return err
	}
	return nil
}

func ensureMySQLUsersPhoneColumn(ctx context.Context, db *sql.DB) error {
	var count int
	err := db.QueryRowContext(ctx, `
		SELECT COUNT(*)
		FROM information_schema.COLUMNS
		WHERE TABLE_SCHEMA = DATABASE() AND TABLE_NAME = 'users' AND COLUMN_NAME = 'phone'`,
	).Scan(&count)
	if err != nil {
		return fmt.Errorf("check users.phone column: %w", err)
	}
	if count > 0 {
		return nil
	}
	_, err = db.ExecContext(ctx, `ALTER TABLE users ADD COLUMN phone VARCHAR(32) NOT NULL DEFAULT '' AFTER signature`)
	if err != nil {
		return fmt.Errorf("add users.phone column: %w", err)
	}
	return nil
}

func ensureMySQLUsersTokenVersionColumn(ctx context.Context, db *sql.DB) error {
	var count int
	err := db.QueryRowContext(ctx, `
		SELECT COUNT(*)
		FROM information_schema.COLUMNS
		WHERE TABLE_SCHEMA = DATABASE() AND TABLE_NAME = 'users' AND COLUMN_NAME = 'token_version'`,
	).Scan(&count)
	if err != nil {
		return fmt.Errorf("check users.token_version column: %w", err)
	}
	if count > 0 {
		_, err = db.ExecContext(ctx, `ALTER TABLE users MODIFY COLUMN token_version BIGINT NOT NULL DEFAULT 1`)
		if err != nil {
			return fmt.Errorf("modify users.token_version column: %w", err)
		}
		return nil
	}
	_, err = db.ExecContext(ctx, `ALTER TABLE users ADD COLUMN token_version BIGINT NOT NULL DEFAULT 1 AFTER client_language`)
	if err != nil {
		return fmt.Errorf("add users.token_version column: %w", err)
	}
	return nil
}

func ensureMySQLTrackScreenshotURLColumn(ctx context.Context, db *sql.DB) error {
	var count int
	err := db.QueryRowContext(ctx, `
		SELECT COUNT(*)
		FROM information_schema.COLUMNS
		WHERE TABLE_SCHEMA = DATABASE() AND TABLE_NAME = 'track_records' AND COLUMN_NAME = 'track_screenshot_url'`,
	).Scan(&count)
	if err != nil {
		return fmt.Errorf("check track_records.track_screenshot_url column: %w", err)
	}
	if count > 0 {
		return nil
	}
	err = db.QueryRowContext(ctx, `
		SELECT COUNT(*)
		FROM information_schema.COLUMNS
		WHERE TABLE_SCHEMA = DATABASE() AND TABLE_NAME = 'track_records' AND COLUMN_NAME = 'screenshot_url'`,
	).Scan(&count)
	if err != nil {
		return fmt.Errorf("check track_records.screenshot_url column: %w", err)
	}
	if count > 0 {
		_, err = db.ExecContext(ctx, `ALTER TABLE track_records CHANGE COLUMN screenshot_url track_screenshot_url VARCHAR(255) COMMENT '轨迹截图文件在对象存储中的地址'`)
		if err != nil {
			return fmt.Errorf("rename track_records.screenshot_url column: %w", err)
		}
		return nil
	}
	_, err = db.ExecContext(ctx, `ALTER TABLE track_records ADD COLUMN track_screenshot_url VARCHAR(255) COMMENT '轨迹截图文件在对象存储中的地址' AFTER raw_track_url`)
	if err != nil {
		return fmt.Errorf("add track_records.track_screenshot_url column: %w", err)
	}
	return nil
}

func ensureMySQLTrackNoMapBgScreenshotURLColumn(ctx context.Context, db *sql.DB) error {
	var count int
	err := db.QueryRowContext(ctx, `
		SELECT COUNT(*)
		FROM information_schema.COLUMNS
		WHERE TABLE_SCHEMA = DATABASE() AND TABLE_NAME = 'track_records' AND COLUMN_NAME = 'track_no_map_bg_screenshot_url'`,
	).Scan(&count)
	if err != nil {
		return fmt.Errorf("check track_records.track_no_map_bg_screenshot_url column: %w", err)
	}
	if count > 0 {
		return nil
	}
	_, err = db.ExecContext(ctx, `ALTER TABLE track_records ADD COLUMN track_no_map_bg_screenshot_url VARCHAR(255) COMMENT '没有地图背景的轨迹路线截图URI' AFTER track_screenshot_url`)
	if err != nil {
		return fmt.Errorf("add track_records.track_no_map_bg_screenshot_url column: %w", err)
	}
	return nil
}

func ensureMySQLTrackLocateAddrColumn(ctx context.Context, db *sql.DB) error {
	var count int
	var maxLength sql.NullInt64
	err := db.QueryRowContext(ctx, `
		SELECT COUNT(*), MAX(CHARACTER_MAXIMUM_LENGTH)
		FROM information_schema.COLUMNS
		WHERE TABLE_SCHEMA = DATABASE() AND TABLE_NAME = 'track_records' AND COLUMN_NAME = 'locate_addr'`,
	).Scan(&count, &maxLength)
	if err != nil {
		return fmt.Errorf("check track_records.locate_addr column: %w", err)
	}
	if count > 0 {
		if maxLength.Valid && maxLength.Int64 < 255 {
			if _, err := db.ExecContext(ctx, `ALTER TABLE track_records MODIFY COLUMN locate_addr VARCHAR(255) NOT NULL DEFAULT '' COMMENT '轨迹的具体位置信息'`); err != nil {
				return fmt.Errorf("modify track_records.locate_addr column: %w", err)
			}
		}
		return nil
	}
	_, err = db.ExecContext(ctx, `ALTER TABLE track_records ADD COLUMN locate_addr VARCHAR(255) NOT NULL DEFAULT '' COMMENT '轨迹的具体位置信息' AFTER city_code`)
	if err != nil {
		return fmt.Errorf("add track_records.locate_addr column: %w", err)
	}
	return nil
}

func ensureMySQLTrackDeletedAtColumn(ctx context.Context, db *sql.DB) error {
	var count int
	err := db.QueryRowContext(ctx, `
		SELECT COUNT(*)
		FROM information_schema.COLUMNS
		WHERE TABLE_SCHEMA = DATABASE() AND TABLE_NAME = 'track_records' AND COLUMN_NAME = 'deleted_at'`,
	).Scan(&count)
	if err != nil {
		return fmt.Errorf("check track_records.deleted_at column: %w", err)
	}
	if count > 0 {
		return nil
	}
	_, err = db.ExecContext(ctx, `ALTER TABLE track_records ADD COLUMN deleted_at DATETIME NULL COMMENT '删除时间' AFTER updated_at`)
	if err != nil {
		return fmt.Errorf("add track_records.deleted_at column: %w", err)
	}
	return nil
}

func ensureMySQLTrackIsRunningColumn(ctx context.Context, db *sql.DB) error {
	var count int
	err := db.QueryRowContext(ctx, `
		SELECT COUNT(*)
		FROM information_schema.COLUMNS
		WHERE TABLE_SCHEMA = DATABASE() AND TABLE_NAME = 'track_records' AND COLUMN_NAME = 'is_running'`,
	).Scan(&count)
	if err != nil {
		return fmt.Errorf("check track_records.is_running column: %w", err)
	}
	if count > 0 {
		_, err = db.ExecContext(ctx, `ALTER TABLE track_records MODIFY COLUMN is_running TINYINT(1) NOT NULL DEFAULT 1`)
		if err != nil {
			return fmt.Errorf("modify track_records.is_running column: %w", err)
		}
		return nil
	}
	_, err = db.ExecContext(ctx, `ALTER TABLE track_records ADD COLUMN is_running TINYINT(1) NOT NULL DEFAULT 1 AFTER track_screenshot_url`)
	if err != nil {
		return fmt.Errorf("add track_records.is_running column: %w", err)
	}
	return nil
}

func ensureMySQLTrackAvgSpeedKmhColumn(ctx context.Context, db *sql.DB) error {
	var count int
	err := db.QueryRowContext(ctx, `
		SELECT COUNT(*)
		FROM information_schema.COLUMNS
		WHERE TABLE_SCHEMA = DATABASE() AND TABLE_NAME = 'track_records' AND COLUMN_NAME = 'avg_speed_kmh'`,
	).Scan(&count)
	if err != nil {
		return fmt.Errorf("check track_records.avg_speed_kmh column: %w", err)
	}
	if count > 0 {
		_, err = db.ExecContext(ctx, `ALTER TABLE track_records MODIFY COLUMN avg_speed_kmh DOUBLE NOT NULL DEFAULT 1`)
		if err != nil {
			return fmt.Errorf("modify track_records.avg_speed_kmh column: %w", err)
		}
		return nil
	}
	_, err = db.ExecContext(ctx, `ALTER TABLE track_records ADD COLUMN avg_speed_kmh DOUBLE NOT NULL DEFAULT 1 AFTER status`)
	if err != nil {
		return fmt.Errorf("add track_records.avg_speed_kmh column: %w", err)
	}
	return nil
}

func ensureMySQLTrackCaloriesBurnedColumn(ctx context.Context, db *sql.DB) error {
	var count int
	err := db.QueryRowContext(ctx, `
		SELECT COUNT(*)
		FROM information_schema.COLUMNS
		WHERE TABLE_SCHEMA = DATABASE() AND TABLE_NAME = 'track_records' AND COLUMN_NAME = 'calories_burned'`,
	).Scan(&count)
	if err != nil {
		return fmt.Errorf("check track_records.calories_burned column: %w", err)
	}
	if count > 0 {
		_, err = db.ExecContext(ctx, `ALTER TABLE track_records MODIFY COLUMN calories_burned DECIMAL(10,2) DEFAULT 0.00 COMMENT '热量消耗(千卡)'`)
		if err != nil {
			return fmt.Errorf("modify track_records.calories_burned column: %w", err)
		}
		return nil
	}
	_, err = db.ExecContext(ctx, `ALTER TABLE track_records ADD COLUMN calories_burned DECIMAL(10,2) DEFAULT 0.00 COMMENT '热量消耗(千卡)' AFTER duration`)
	if err != nil {
		return fmt.Errorf("add track_records.calories_burned column: %w", err)
	}
	return nil
}

func ensureMySQLTrackCityCodeColumn(ctx context.Context, db *sql.DB) error {
	var count int
	err := db.QueryRowContext(ctx, `
		SELECT COUNT(*)
		FROM information_schema.COLUMNS
		WHERE TABLE_SCHEMA = DATABASE() AND TABLE_NAME = 'track_records' AND COLUMN_NAME = 'city_code'`,
	).Scan(&count)
	if err != nil {
		return fmt.Errorf("check track_records.city_code column: %w", err)
	}
	if count > 0 {
		return nil
	}
	// city_code 作为“轨迹归属城市”字段，客户端创建轨迹时传入。
	_, err = db.ExecContext(ctx, `ALTER TABLE track_records ADD COLUMN city_code VARCHAR(16) NOT NULL DEFAULT '' COMMENT '城市Code' AFTER user_id`)
	if err != nil {
		return fmt.Errorf("add track_records.city_code column: %w", err)
	}
	return nil
}

func ensureMySQLTrackSessionIDColumn(ctx context.Context, db *sql.DB) error {
	var count int
	err := db.QueryRowContext(ctx, `
		SELECT COUNT(*)
		FROM information_schema.COLUMNS
		WHERE TABLE_SCHEMA = DATABASE() AND TABLE_NAME = 'track_records' AND COLUMN_NAME = 'session_id'`,
	).Scan(&count)
	if err != nil {
		return fmt.Errorf("check track_records.session_id column: %w", err)
	}
	if count > 0 {
		if _, err = db.ExecContext(ctx, `UPDATE track_records SET session_id='' WHERE session_id IS NULL`); err != nil {
			return fmt.Errorf("backfill track_records.session_id column: %w", err)
		}
		_, err = db.ExecContext(ctx, `ALTER TABLE track_records MODIFY COLUMN session_id VARCHAR(64) NOT NULL DEFAULT '' COMMENT '关联的同行会话ID'`)
		if err != nil {
			return fmt.Errorf("modify track_records.session_id column: %w", err)
		}
		return nil
	}
	_, err = db.ExecContext(ctx, `ALTER TABLE track_records ADD COLUMN session_id VARCHAR(64) NOT NULL DEFAULT '' COMMENT '关联的同行会话ID' AFTER user_id`)
	if err != nil {
		return fmt.Errorf("add track_records.session_id column: %w", err)
	}
	return nil
}

func ensureMySQLTrackSessionIDIndex(ctx context.Context, db *sql.DB) error {
	var count int
	err := db.QueryRowContext(ctx, `
		SELECT COUNT(*)
		FROM information_schema.STATISTICS
		WHERE TABLE_SCHEMA = DATABASE() AND TABLE_NAME = 'track_records' AND INDEX_NAME = 'idx_track_session'`,
	).Scan(&count)
	if err != nil {
		return fmt.Errorf("check track_records.idx_track_session index: %w", err)
	}
	if count > 0 {
		return nil
	}
	_, err = db.ExecContext(ctx, `ALTER TABLE track_records ADD INDEX idx_track_session (session_id)`)
	if err != nil {
		return fmt.Errorf("add track_records.idx_track_session index: %w", err)
	}
	return nil
}

func ensureMySQLTrackTypeColumn(ctx context.Context, db *sql.DB) error {
	var count int
	err := db.QueryRowContext(ctx, `
		SELECT COUNT(*)
		FROM information_schema.COLUMNS
		WHERE TABLE_SCHEMA = DATABASE() AND TABLE_NAME = 'track_records' AND COLUMN_NAME = 'track_type'`,
	).Scan(&count)
	if err != nil {
		return fmt.Errorf("check track_records.track_type column: %w", err)
	}
	if count > 0 {
		return nil
	}
	_, err = db.ExecContext(ctx, `ALTER TABLE track_records ADD COLUMN track_type VARCHAR(32) NOT NULL DEFAULT '' COMMENT '轨迹类型' AFTER city_code`)
	if err != nil {
		return fmt.Errorf("add track_records.track_type column: %w", err)
	}
	return nil
}

func ensureMySQLTrackSourceTagColumn(ctx context.Context, db *sql.DB) error {
	var count int
	err := db.QueryRowContext(ctx, `
		SELECT COUNT(*)
		FROM information_schema.COLUMNS
		WHERE TABLE_SCHEMA = DATABASE() AND TABLE_NAME = 'track_records' AND COLUMN_NAME = 'source_tag'`,
	).Scan(&count)
	if err != nil {
		return fmt.Errorf("check track_records.source_tag column: %w", err)
	}
	if count > 0 {
		return nil
	}
	_, err = db.ExecContext(ctx, `ALTER TABLE track_records ADD COLUMN source_tag VARCHAR(64) NOT NULL DEFAULT '' COMMENT '轨迹来源/运营标签' AFTER track_type`)
	if err != nil {
		return fmt.Errorf("add track_records.source_tag column: %w", err)
	}
	return nil
}

func ensureMySQLTrackSourceTagIndex(ctx context.Context, db *sql.DB) error {
	var count int
	err := db.QueryRowContext(ctx, `
		SELECT COUNT(*)
		FROM information_schema.STATISTICS
		WHERE TABLE_SCHEMA = DATABASE() AND TABLE_NAME = 'track_records' AND INDEX_NAME = 'idx_track_source_tag'`,
	).Scan(&count)
	if err != nil {
		return fmt.Errorf("check track_records.idx_track_source_tag index: %w", err)
	}
	if count > 0 {
		return nil
	}
	_, err = db.ExecContext(ctx, `ALTER TABLE track_records ADD INDEX idx_track_source_tag (source_tag)`)
	if err != nil {
		return fmt.Errorf("add track_records.idx_track_source_tag index: %w", err)
	}
	return nil
}

func ensureMySQLCompanionSessionTrackTypeColumn(ctx context.Context, db *sql.DB) error {
	var count int
	err := db.QueryRowContext(ctx, `
		SELECT COUNT(*)
		FROM information_schema.COLUMNS
		WHERE TABLE_SCHEMA = DATABASE() AND TABLE_NAME = 'companion_sessions' AND COLUMN_NAME = 'track_type'`,
	).Scan(&count)
	if err != nil {
		return fmt.Errorf("check companion_sessions.track_type column: %w", err)
	}
	if count > 0 {
		return nil
	}
	_, err = db.ExecContext(ctx, `ALTER TABLE companion_sessions ADD COLUMN track_type VARCHAR(32) NOT NULL DEFAULT '' COMMENT '运动类型' AFTER title`)
	if err != nil {
		return fmt.Errorf("add companion_sessions.track_type column: %w", err)
	}
	return nil
}

func ensureMySQLCompanionSessionLocateAddrColumn(ctx context.Context, db *sql.DB) error {
	var count int
	err := db.QueryRowContext(ctx, `
		SELECT COUNT(*)
		FROM information_schema.COLUMNS
		WHERE TABLE_SCHEMA = DATABASE() AND TABLE_NAME = 'companion_sessions' AND COLUMN_NAME = 'locate_addr'`,
	).Scan(&count)
	if err != nil {
		return fmt.Errorf("check companion_sessions.locate_addr column: %w", err)
	}
	if count > 0 {
		return nil
	}
	_, err = db.ExecContext(ctx, `ALTER TABLE companion_sessions ADD COLUMN locate_addr VARCHAR(255) NOT NULL DEFAULT '' COMMENT '位置信息' AFTER track_type`)
	if err != nil {
		return fmt.Errorf("add companion_sessions.locate_addr column: %w", err)
	}
	return nil
}

func ensureMySQLCompanionSessionVisibilityColumn(ctx context.Context, db *sql.DB) error {
	var count int
	err := db.QueryRowContext(ctx, `
		SELECT COUNT(*)
		FROM information_schema.COLUMNS
		WHERE TABLE_SCHEMA = DATABASE() AND TABLE_NAME = 'companion_sessions' AND COLUMN_NAME = 'visibility'`,
	).Scan(&count)
	if err != nil {
		return fmt.Errorf("check companion_sessions.visibility column: %w", err)
	}
	if count > 0 {
		return nil
	}
	_, err = db.ExecContext(ctx, `ALTER TABLE companion_sessions ADD COLUMN visibility VARCHAR(16) NOT NULL DEFAULT 'private' COMMENT 'private / public' AFTER status`)
	if err != nil {
		return fmt.Errorf("add companion_sessions.visibility column: %w", err)
	}
	return nil
}

func ensureMySQLCompanionSessionDanmakuEnabledColumn(ctx context.Context, db *sql.DB) error {
	var count int
	err := db.QueryRowContext(ctx, `
		SELECT COUNT(*)
		FROM information_schema.COLUMNS
		WHERE TABLE_SCHEMA = DATABASE() AND TABLE_NAME = 'companion_sessions' AND COLUMN_NAME = 'danmaku_enabled'`,
	).Scan(&count)
	if err != nil {
		return fmt.Errorf("check companion_sessions.danmaku_enabled column: %w", err)
	}
	if count > 0 {
		return nil
	}
	_, err = db.ExecContext(ctx, `ALTER TABLE companion_sessions ADD COLUMN danmaku_enabled TINYINT(1) NOT NULL DEFAULT 1 COMMENT '弹幕开关：1=开启 0=关闭' AFTER max_members`)
	if err != nil {
		return fmt.Errorf("add companion_sessions.danmaku_enabled column: %w", err)
	}
	return nil
}

func ensureMySQLCompanionSessionStatsColumns(ctx context.Context, db *sql.DB) error {
	columns := []struct {
		name       string
		definition string
	}{
		{
			name:       "total_distance",
			definition: "ALTER TABLE companion_sessions ADD COLUMN total_distance DOUBLE NOT NULL DEFAULT 0 COMMENT '同行总里程，单位米' AFTER danmaku_enabled",
		},
		{
			name:       "total_duration",
			definition: "ALTER TABLE companion_sessions ADD COLUMN total_duration BIGINT NOT NULL DEFAULT 0 COMMENT '同行总耗时，单位秒' AFTER total_distance",
		},
		{
			name:       "track_screenshot_url",
			definition: "ALTER TABLE companion_sessions ADD COLUMN track_screenshot_url VARCHAR(255) NOT NULL DEFAULT '' COMMENT '同行轨迹截图文件在对象存储中的地址' AFTER total_duration",
		},
		{
			name:       "actual_participant_count",
			definition: "ALTER TABLE companion_sessions ADD COLUMN actual_participant_count BIGINT NOT NULL DEFAULT 0 COMMENT '实际参与同行人数' AFTER track_screenshot_url",
		},
	}
	for _, column := range columns {
		var count int
		err := db.QueryRowContext(ctx, `
			SELECT COUNT(*)
			FROM information_schema.COLUMNS
			WHERE TABLE_SCHEMA = DATABASE() AND TABLE_NAME = 'companion_sessions' AND COLUMN_NAME = ?`,
			column.name,
		).Scan(&count)
		if err != nil {
			return fmt.Errorf("check companion_sessions.%s column: %w", column.name, err)
		}
		if count > 0 {
			continue
		}
		if _, err := db.ExecContext(ctx, column.definition); err != nil {
			return fmt.Errorf("add companion_sessions.%s column: %w", column.name, err)
		}
	}
	return nil
}

func ensureMySQLCompanionSessionEndAuditColumns(ctx context.Context, db *sql.DB) error {
	columns := []struct {
		name       string
		definition string
	}{
		{
			name:       "end_reason",
			definition: "ALTER TABLE companion_sessions ADD COLUMN end_reason VARCHAR(32) NOT NULL DEFAULT '' COMMENT '结束原因：owner_ended / all_members_left / inactive_timeout / max_duration_exceeded' AFTER ended_at",
		},
		{
			name:       "end_source",
			definition: "ALTER TABLE companion_sessions ADD COLUMN end_source VARCHAR(32) NOT NULL DEFAULT '' COMMENT '结束来源：owner / member_flow / auto_close' AFTER end_reason",
		},
		{
			name:       "end_operator_user_id",
			definition: "ALTER TABLE companion_sessions ADD COLUMN end_operator_user_id BIGINT NOT NULL DEFAULT 0 COMMENT '结束操作用户ID；自动收尾为0' AFTER end_source",
		},
	}
	for _, column := range columns {
		var count int
		err := db.QueryRowContext(ctx, `
			SELECT COUNT(*)
			FROM information_schema.COLUMNS
			WHERE TABLE_SCHEMA = DATABASE() AND TABLE_NAME = 'companion_sessions' AND COLUMN_NAME = ?`,
			column.name,
		).Scan(&count)
		if err != nil {
			return fmt.Errorf("check companion_sessions.%s column: %w", column.name, err)
		}
		if count > 0 {
			continue
		}
		if _, err := db.ExecContext(ctx, column.definition); err != nil {
			return fmt.Errorf("add companion_sessions.%s column: %w", column.name, err)
		}
	}
	return nil
}

// ensureMySQLCompanionSessionStatusEndedIndex 在已有部署上补齐 (status, ended_at) 复合索引，
// 用于支持「弹幕清理」按已结束会话过滤的扫描。
func ensureMySQLCompanionSessionStatusEndedIndex(ctx context.Context, db *sql.DB) error {
	var count int
	err := db.QueryRowContext(ctx, `
		SELECT COUNT(*)
		FROM information_schema.STATISTICS
		WHERE TABLE_SCHEMA = DATABASE() AND TABLE_NAME = 'companion_sessions' AND INDEX_NAME = 'idx_companion_session_status_ended'`,
	).Scan(&count)
	if err != nil {
		return fmt.Errorf("check companion_sessions.idx_companion_session_status_ended: %w", err)
	}
	if count > 0 {
		return nil
	}
	_, err = db.ExecContext(ctx, `ALTER TABLE companion_sessions ADD INDEX idx_companion_session_status_ended (status, ended_at)`)
	if err != nil {
		return fmt.Errorf("add companion_sessions.idx_companion_session_status_ended: %w", err)
	}
	return nil
}

func ensureMySQLUserIDColumnsBigint(ctx context.Context, db *sql.DB) error {
	columns := []struct {
		table      string
		column     string
		columnType string
		definition string
	}{
		{table: "users", column: "id", columnType: "bigint", definition: "BIGINT NOT NULL"},
		{table: "track_records", column: "user_id", columnType: "bigint unsigned", definition: "BIGINT UNSIGNED NOT NULL"},
		{table: "track_waypoints", column: "user_id", columnType: "bigint unsigned", definition: "BIGINT UNSIGNED NOT NULL"},
		{table: "track_collects", column: "user_id", columnType: "bigint", definition: "BIGINT NOT NULL"},
	}
	for _, item := range columns {
		if err := ensureMySQLBigintColumn(ctx, db, item.table, item.column, item.columnType, item.definition); err != nil {
			return err
		}
	}
	return nil
}

func ensureMySQLTrackCoordinateSystemColumn(ctx context.Context, db *sql.DB) error {
	var count int
	err := db.QueryRowContext(ctx, `
		SELECT COUNT(*)
		FROM information_schema.COLUMNS
		WHERE TABLE_SCHEMA = DATABASE() AND TABLE_NAME = 'track_records' AND COLUMN_NAME = 'coordinate_system'`,
	).Scan(&count)
	if err != nil {
		return fmt.Errorf("check track_records.coordinate_system column: %w", err)
	}
	if count > 0 {
		return nil
	}
	_, err = db.ExecContext(ctx, `ALTER TABLE track_records ADD COLUMN coordinate_system VARCHAR(32) NOT NULL DEFAULT '' COMMENT '坐标系' AFTER track_type`)
	if err != nil {
		return fmt.Errorf("add track_records.coordinate_system column: %w", err)
	}
	return nil
}

func ensureMySQLTrackWaypointTrackIDColumn(ctx context.Context, db *sql.DB) error {
	var columnType string
	err := db.QueryRowContext(ctx, `
		SELECT COLUMN_TYPE
		FROM information_schema.COLUMNS
		WHERE TABLE_SCHEMA = DATABASE() AND TABLE_NAME = 'track_waypoints' AND COLUMN_NAME = 'track_id'`,
	).Scan(&columnType)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil
		}
		return fmt.Errorf("check track_waypoints.track_id type: %w", err)
	}
	if strings.EqualFold(columnType, "varchar(128)") {
		return nil
	}
	_, err = db.ExecContext(ctx, `ALTER TABLE track_waypoints MODIFY COLUMN track_id VARCHAR(128) NOT NULL COMMENT '关联的轨迹主表ID'`)
	if err != nil {
		return fmt.Errorf("alter track_waypoints.track_id to VARCHAR(128): %w", err)
	}
	return nil
}

func ensureMySQLBigintColumn(ctx context.Context, db *sql.DB, tableName, columnName, expectedColumnType, definition string) error {
	var dataType, columnType string
	err := db.QueryRowContext(ctx, `
		SELECT DATA_TYPE, COLUMN_TYPE
		FROM information_schema.COLUMNS
		WHERE TABLE_SCHEMA = DATABASE() AND TABLE_NAME = ? AND COLUMN_NAME = ?`,
		tableName, columnName,
	).Scan(&dataType, &columnType)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil
		}
		return fmt.Errorf("check %s.%s type: %w", tableName, columnName, err)
	}
	if strings.EqualFold(columnType, expectedColumnType) {
		return nil
	}
	if err := ensureMySQLColumnConvertibleToBigint(ctx, db, tableName, columnName, dataType); err != nil {
		return err
	}
	_, err = db.ExecContext(ctx, fmt.Sprintf("ALTER TABLE %s MODIFY COLUMN %s %s", tableName, columnName, definition))
	if err != nil {
		return fmt.Errorf("alter %s.%s to %s: %w", tableName, columnName, expectedColumnType, err)
	}
	return nil
}

func ensureMySQLColumnConvertibleToBigint(ctx context.Context, db *sql.DB, tableName, columnName, dataType string) error {
	switch dataType {
	case "varchar", "char", "text", "tinytext", "mediumtext", "longtext":
		var invalidCount int
		query := fmt.Sprintf("SELECT COUNT(*) FROM %s WHERE %s = '' OR %s NOT REGEXP '^[0-9]+$'", tableName, columnName, columnName)
		if err := db.QueryRowContext(ctx, query).Scan(&invalidCount); err != nil {
			return fmt.Errorf("check %s.%s convertible to BIGINT: %w", tableName, columnName, err)
		}
		if invalidCount > 0 {
			return fmt.Errorf("%s.%s contains non-numeric values, cannot migrate to BIGINT", tableName, columnName)
		}
	}
	return nil
}

func nullableStringValue(value string) interface{} {
	if value == "" {
		return nil
	}
	return value
}

func nullableTimeValue(value time.Time) interface{} {
	if value.IsZero() {
		return nil
	}
	return value
}

// MySQLTrackRepository implements TrackRepository on top of MySQL.
type MySQLTrackRepository struct{ db *sql.DB }

// MySQLTrackWaypointRepository implements TrackWaypointRepository on top of MySQL.
type MySQLTrackWaypointRepository struct{ db *sql.DB }

func NewMySQLTrackRepository(db *sql.DB) *MySQLTrackRepository { return &MySQLTrackRepository{db: db} }

// NextTrackID 使用独立的 MySQL 自增序列表分配全局唯一序列号，再编码为业务轨迹 ID。
// 这样做有两个好处：
//  1. 唯一性由数据库自增机制保证，不依赖进程内计数器或时间戳；
//  2. 轨迹主表 `track_records` 不需要承担“生成序列”的职责，职责更清晰。
//
// 这里故意使用单独的 `track_id_sequences` 表，而不是复用 `track_records` 主键，
// 是因为轨迹记录可能受事务、重试、软删除等业务流程影响；独立序列表更适合作为稳定的全局发号器。
func (r *MySQLTrackRepository) NextTrackID(ctx context.Context) (string, error) {
	res, err := r.db.ExecContext(ctx, `INSERT INTO track_id_sequences (id) VALUES (NULL)`)
	if err != nil {
		return "", err
	}
	sequence, err := res.LastInsertId()
	if err != nil {
		return "", err
	}
	return encodeTrackID(uint64(sequence))
}

func NewMySQLTrackWaypointRepository(db *sql.DB) *MySQLTrackWaypointRepository {
	return &MySQLTrackWaypointRepository{db: db}
}

func (r *MySQLTrackRepository) Create(ctx context.Context, t *models.Track) error {
	if t.ID == "" {
		return errors.New("track id is required")
	}
	if t.CreatedAt.IsZero() {
		t.CreatedAt = time.Now()
	}
	if t.StartTime.IsZero() {
		t.StartTime = t.CreatedAt
	}
	t.UpdatedAt = t.CreatedAt

	_, err := r.db.ExecContext(ctx,
		`INSERT INTO track_records (
			id, user_id, session_id, city_code, locate_addr, track_type, source_tag, coordinate_system, title, start_time, end_time,
			distance, duration, calories_burned, elevation_gain, raw_track_url, track_screenshot_url, track_no_map_bg_screenshot_url, is_running, status, avg_speed_kmh,
			created_at, updated_at, deleted_at
		) VALUES (?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?)`,
		t.ID, t.UserID, t.SessionID, t.CityCode, t.LocateAddr, t.TrackType, t.SourceTag, t.CoordinateSystem, t.Title, t.StartTime, nullableTimeValue(t.EndTime),
		t.Distance, t.Duration, t.CaloriesBurned, t.ElevationGain, nullableStringValue(t.RawTrackURL), nullableStringValue(t.TrackScreenshotURL), nullableStringValue(t.TrackNoMapBgScreenshotURL), t.IsRunning, t.Status, t.AvgSpeedKmh,
		t.CreatedAt, t.UpdatedAt, nullableTimeValue(t.DeletedAt),
	)
	if err != nil {
		var mysqlErr *mysql.MySQLError
		if errors.As(err, &mysqlErr) && mysqlErr.Number == 1062 {
			return ErrAlreadyExists
		}
		return err
	}
	return nil
}

func (r *MySQLTrackRepository) Update(ctx context.Context, t *models.Track) error {
	t.UpdatedAt = time.Now()

	res, err := r.db.ExecContext(ctx,
		`UPDATE track_records SET
			user_id=?, session_id=?, city_code=?, locate_addr=?, track_type=?, source_tag=?, coordinate_system=?, title=?, start_time=?, end_time=?,
			distance=?, duration=?, calories_burned=?, elevation_gain=?, raw_track_url=?, track_screenshot_url=?, track_no_map_bg_screenshot_url=?, is_running=?, status=?, avg_speed_kmh=?, updated_at=?, deleted_at=?
		WHERE id=?`,
		t.UserID, t.SessionID, t.CityCode, t.LocateAddr, t.TrackType, t.SourceTag, t.CoordinateSystem, t.Title, t.StartTime, nullableTimeValue(t.EndTime),
		t.Distance, t.Duration, t.CaloriesBurned, t.ElevationGain, nullableStringValue(t.RawTrackURL), nullableStringValue(t.TrackScreenshotURL), nullableStringValue(t.TrackNoMapBgScreenshotURL), t.IsRunning, t.Status, t.AvgSpeedKmh, t.UpdatedAt,
		nullableTimeValue(t.DeletedAt),
		t.ID,
	)
	if err != nil {
		return err
	}
	rows, _ := res.RowsAffected()
	if rows == 0 {
		return ErrNotFound
	}
	return nil
}

// SoftDeleteAndCleanupCollectsTx 在一个 MySQL 事务中完成“删除轨迹 + 清理收藏记录”。
//
// 为什么要用事务：
// - 轨迹删除是软删除（track_records.status=0 + deleted_at），同时需要清理 track_collects；
// - 若不使用事务，可能出现“轨迹已删除但收藏未清理 / 收藏已清理但轨迹未删除”的中间态；
// - 使用事务可以保证这两步要么都成功提交，要么都回滚。
//
// 事务内做的事情：
// 1) 先 SELECT 校验归属（track_records.user_id 必须等于 userID），不允许删他人轨迹。
// 2) UPDATE 软删除 track_records：status=0、is_running=0、deleted_at（若已存在则保持不变）、updated_at。
// 3) DELETE 清理 track_collects：删除所有 track_id=该轨迹的收藏关系（不区分收藏用户）。
func (r *MySQLTrackRepository) SoftDeleteAndCleanupCollectsTx(ctx context.Context, userID int64, trackID string) error {
	if userID <= 0 || trackID == "" {
		return ErrNotFound
	}
	// 显式开启事务；后续任一步失败都会触发 defer 的 Rollback。
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	// 注意：这里统一 defer Rollback，Commit 成功后 Rollback 会返回 sql.ErrTxDone 并被忽略。
	defer func() { _ = tx.Rollback() }()

	var (
		ownerUserID int64
		deletedAt   sql.NullTime
	)
	// 1) 读取 owner_user_id 做权限校验。
	if err := tx.QueryRowContext(ctx, `SELECT user_id, deleted_at FROM track_records WHERE id=? LIMIT 1`, trackID).Scan(&ownerUserID, &deletedAt); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return ErrNotFound
		}
		return err
	}
	if ownerUserID != userID {
		return ErrForbidden
	}

	now := time.Now()
	da := now
	if deletedAt.Valid {
		// 若之前已删除过，deleted_at 保持原值，避免重复删除刷新时间导致审计/排序混乱。
		da = deletedAt.Time
	}
	// 2) 软删除轨迹。
	if _, err := tx.ExecContext(ctx,
		`UPDATE track_records SET status=?, is_running=0, deleted_at=?, updated_at=? WHERE id=?`,
		models.TrackStatusDeleted, da, now, trackID,
	); err != nil {
		return err
	}
	// 3) 清理收藏关系。
	if _, err := tx.ExecContext(ctx, `DELETE FROM track_collects WHERE track_id=?`, trackID); err != nil {
		return err
	}
	// 事务提交：只有 Commit 成功，删除与清理才对外可见。
	return tx.Commit()
}

func (r *MySQLTrackRepository) FindByID(ctx context.Context, id string) (*models.Track, error) {
	row := r.db.QueryRowContext(ctx,
		`SELECT id, user_id, session_id, city_code, locate_addr, track_type, source_tag, coordinate_system, title, start_time, end_time,
			distance, duration, calories_burned, elevation_gain, raw_track_url, track_screenshot_url, track_no_map_bg_screenshot_url, is_running, status, avg_speed_kmh,
			created_at, updated_at, deleted_at
		FROM track_records WHERE id=?`, id)

	var (
		t               models.Track
		endTime         sql.NullTime
		deletedAt       sql.NullTime
		distance        sql.NullFloat64
		duration        sql.NullInt64
		caloriesBurned  sql.NullFloat64
		elevationGain   sql.NullInt64
		rawTrackURL     sql.NullString
		trackScreenshot sql.NullString
		noMapBgShot     sql.NullString
		coordinateSys   sql.NullString
	)
	if err := row.Scan(
		&t.ID, &t.UserID, &t.SessionID, &t.CityCode, &t.LocateAddr, &t.TrackType, &t.SourceTag, &coordinateSys, &t.Title, &t.StartTime, &endTime,
		&distance, &duration, &caloriesBurned, &elevationGain, &rawTrackURL, &trackScreenshot, &noMapBgShot, &t.IsRunning, &t.Status, &t.AvgSpeedKmh,
		&t.CreatedAt, &t.UpdatedAt, &deletedAt,
	); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, err
	}
	if endTime.Valid {
		t.EndTime = endTime.Time
	}
	if deletedAt.Valid {
		t.DeletedAt = deletedAt.Time
	}
	if distance.Valid {
		t.Distance = distance.Float64
	}
	if duration.Valid {
		t.Duration = uint32(duration.Int64)
	}
	if caloriesBurned.Valid {
		t.CaloriesBurned = caloriesBurned.Float64
	}
	if elevationGain.Valid {
		t.ElevationGain = int(elevationGain.Int64)
	}
	if rawTrackURL.Valid {
		t.RawTrackURL = rawTrackURL.String
	}
	if trackScreenshot.Valid {
		t.TrackScreenshotURL = trackScreenshot.String
	}
	if noMapBgShot.Valid {
		t.TrackNoMapBgScreenshotURL = noMapBgShot.String
	}
	if coordinateSys.Valid {
		t.CoordinateSystem = coordinateSys.String
	}
	return &t, nil
}

func (r *MySQLTrackRepository) FindRunningByUserID(ctx context.Context, userID int64) (*models.Track, error) {
	row := r.db.QueryRowContext(ctx,
		`SELECT id FROM track_records WHERE user_id=? AND is_running=1 ORDER BY start_time DESC LIMIT 1`,
		userID,
	)
	var id string
	if err := row.Scan(&id); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, err
	}
	return r.FindByID(ctx, id)
}

// StatsByUserID returns (trackCount, totalDistance) for a user.
// 口径与 ListByUserID 保持一致：排除删除与进行中轨迹；其中 trackCount 仅统计 raw_track_url 非空的轨迹。
func (r *MySQLTrackRepository) StatsByUserID(ctx context.Context, userID int64) (int64, float64, error) {
	row := r.db.QueryRowContext(
		ctx,
		`SELECT COALESCE(SUM(CASE WHEN raw_track_url IS NOT NULL AND raw_track_url <> '' THEN 1 ELSE 0 END), 0) AS cnt,
		        COALESCE(SUM(distance), 0) AS total_distance
		 FROM track_records
		 WHERE user_id=? AND is_running=0 AND status IN (?, ?)`,
		userID, models.TrackStatusNormal, models.TrackStatusPrivate,
	)
	var (
		cnt  int64
		dist float64
	)
	if err := row.Scan(&cnt, &dist); err != nil {
		return 0, 0, err
	}
	return cnt, dist, nil
}

// StatsSummaryByUserID returns user track statistics from track_records.
// 口径：排除删除与进行中轨迹，统计正常/私密轨迹的总里程、次数、总耗时和总热量。
func (r *MySQLTrackRepository) StatsSummaryByUserID(ctx context.Context, userID int64) (*models.TrackUserStats, error) {
	row := r.db.QueryRowContext(
		ctx,
		`SELECT COUNT(*) AS track_count,
		        COALESCE(SUM(distance), 0) AS total_distance,
		        COALESCE(SUM(duration), 0) AS total_duration,
		        COALESCE(SUM(calories_burned), 0) AS total_calories
		 FROM track_records
		 WHERE user_id=? AND is_running=0 AND status IN (?, ?)`,
		userID, models.TrackStatusNormal, models.TrackStatusPrivate,
	)
	stats := &models.TrackUserStats{}
	if err := row.Scan(&stats.TrackCount, &stats.TotalDistance, &stats.TotalDuration, &stats.TotalCalories); err != nil {
		return nil, err
	}
	return stats, nil
}

// CountByUserIDWithNonEmptyRawTrackURL returns track count of a user where raw_track_url is non-empty.
// 口径与 ListByUserID 保持一致：排除删除与进行中轨迹，并且仅统计 raw_track_url 非空。
func (r *MySQLTrackRepository) CountByUserIDWithNonEmptyRawTrackURL(ctx context.Context, userID int64) (int64, error) {
	row := r.db.QueryRowContext(
		ctx,
		`SELECT COUNT(*) AS cnt
		 FROM track_records
		 WHERE user_id=? AND is_running=0 AND status IN (?, ?) AND raw_track_url IS NOT NULL AND raw_track_url <> ''`,
		userID, models.TrackStatusNormal, models.TrackStatusPrivate,
	)
	var cnt int64
	if err := row.Scan(&cnt); err != nil {
		return 0, err
	}
	return cnt, nil
}

func (r *MySQLTrackRepository) ListByUserID(ctx context.Context, userID int64, cursor *models.TrackListCursor, limit int) ([]*models.Track, error) {
	if limit <= 0 {
		limit = 20
	}
	query := `SELECT id FROM track_records WHERE user_id=? AND status IN (?, ?) AND is_running=0`
	args := make([]interface{}, 0, 6)
	args = append(args, userID, models.TrackStatusNormal, models.TrackStatusPrivate)
	if cursor != nil {
		query += ` AND (start_time < ? OR (start_time = ? AND id < ?))`
		args = append(args, cursor.StartTime, cursor.StartTime, cursor.ID)
	}
	query += ` ORDER BY start_time DESC, id DESC LIMIT ?`
	args = append(args, limit)
	rows, err := r.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	ids := make([]string, 0, limit)
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		ids = append(ids, id)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	res := make([]*models.Track, 0, len(ids))
	for _, id := range ids {
		t, err := r.FindByID(ctx, id)
		if err != nil {
			if errors.Is(err, ErrNotFound) {
				continue
			}
			return nil, err
		}
		res = append(res, t)
	}
	return res, nil
}

func (r *MySQLTrackRepository) ListRecommend(ctx context.Context, _ int64, cursor *models.TrackListCursor, limit int) ([]*models.Track, error) {
	if limit <= 0 {
		limit = 20
	}
	query := `SELECT id FROM track_records WHERE status=? AND is_running=0`
	args := make([]interface{}, 0, 4)
	args = append(args, models.TrackStatusNormal)
	if cursor != nil {
		query += ` AND (start_time < ? OR (start_time = ? AND id < ?))`
		args = append(args, cursor.StartTime, cursor.StartTime, cursor.ID)
	}
	query += ` ORDER BY start_time DESC, id DESC LIMIT ?`
	args = append(args, limit)
	rows, err := r.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	ids := make([]string, 0, limit)
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		ids = append(ids, id)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	res := make([]*models.Track, 0, len(ids))
	for _, id := range ids {
		t, err := r.FindByID(ctx, id)
		if err != nil {
			if errors.Is(err, ErrNotFound) {
				continue
			}
			return nil, err
		}
		res = append(res, t)
	}
	return res, nil
}

func (r *MySQLTrackRepository) Search(ctx context.Context, keyword string, cursor *models.TrackListCursor, limit int) ([]*models.Track, error) {
	if limit <= 0 {
		limit = 20
	}
	like := "%" + keyword + "%"
	query := `SELECT id FROM track_records WHERE status=? AND (? = '' OR title LIKE ?)`
	args := make([]interface{}, 0, 6)
	args = append(args, models.TrackStatusNormal, keyword, like)
	if cursor != nil {
		query += ` AND (start_time < ? OR (start_time = ? AND id < ?))`
		args = append(args, cursor.StartTime, cursor.StartTime, cursor.ID)
	}
	query += ` ORDER BY start_time DESC, id DESC LIMIT ?`
	args = append(args, limit)
	rows, err := r.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	ids := make([]string, 0, limit)
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		ids = append(ids, id)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	res := make([]*models.Track, 0, len(ids))
	for _, id := range ids {
		t, err := r.FindByID(ctx, id)
		if err != nil {
			if errors.Is(err, ErrNotFound) {
				continue
			}
			return nil, err
		}
		res = append(res, t)
	}
	return res, nil
}

// ListAll 返回全量未删除轨迹（按 start_time desc, id desc）。仅供管理后台使用。
func (r *MySQLTrackRepository) ListAll(ctx context.Context, cursor *models.TrackListCursor, limit int) ([]*models.Track, error) {
	if limit <= 0 {
		limit = 20
	}
	query := `SELECT id FROM track_records WHERE (deleted_at IS NULL) AND status <> ?`
	args := make([]interface{}, 0, 4)
	args = append(args, models.TrackStatusDeleted)
	if cursor != nil && !cursor.StartTime.IsZero() {
		query += ` AND (start_time < ? OR (start_time = ? AND id < ?))`
		args = append(args, cursor.StartTime, cursor.StartTime, cursor.ID)
	}
	query += ` ORDER BY start_time DESC, id DESC LIMIT ?`
	args = append(args, limit)
	rows, err := r.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	ids := make([]string, 0, limit)
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		ids = append(ids, id)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	res := make([]*models.Track, 0, len(ids))
	for _, id := range ids {
		t, err := r.FindByID(ctx, id)
		if err != nil {
			if errors.Is(err, ErrNotFound) {
				continue
			}
			return nil, err
		}
		res = append(res, t)
	}
	return res, nil
}

// CountAll 返回全量未删除轨迹数量。仅供管理后台使用。
func (r *MySQLTrackRepository) CountAll(ctx context.Context) (int64, error) {
	row := r.db.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM track_records WHERE (deleted_at IS NULL) AND status <> ?`,
		models.TrackStatusDeleted,
	)
	var count int64
	if err := row.Scan(&count); err != nil {
		return 0, err
	}
	return count, nil
}

func (r *MySQLTrackWaypointRepository) Create(ctx context.Context, waypoint *models.TrackWaypoint) error {
	if waypoint.TrackID == "" {
		return errors.New("track waypoint track_id is required")
	}
	if waypoint.CreatedAt.IsZero() {
		waypoint.CreatedAt = time.Now()
	}
	var mediaURLsJSON interface{}
	if waypoint.MediaURLs != nil {
		buf, err := json.Marshal(waypoint.MediaURLs)
		if err != nil {
			return err
		}
		mediaURLsJSON = string(buf)
	}
	res, err := r.db.ExecContext(ctx,
		`INSERT INTO track_waypoints (
			track_id, user_id, lat, lng, elevation,
			node_time, media_type, content, media_urls, created_at
		) VALUES (?,?,?,?,?,?,?,?,?,?)`,
		waypoint.TrackID, waypoint.UserID, waypoint.Lat, waypoint.Lng, waypoint.Elevation,
		waypoint.NodeTime, waypoint.MediaType, nullableStringValue(waypoint.Content), mediaURLsJSON, waypoint.CreatedAt,
	)
	if err != nil {
		return err
	}
	id, err := res.LastInsertId()
	if err != nil {
		return err
	}
	waypoint.ID = uint64(id)
	return nil
}

func (r *MySQLTrackWaypointRepository) ListByTrackID(ctx context.Context, trackID string) ([]*models.TrackWaypoint, error) {
	rows, err := r.db.QueryContext(ctx,
		`SELECT id, track_id, user_id, lat, lng, elevation,
			node_time, media_type, content, media_urls, created_at
		FROM track_waypoints
		WHERE track_id=?
		ORDER BY node_time ASC, id ASC`,
		trackID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	res := make([]*models.TrackWaypoint, 0)
	for rows.Next() {
		var (
			waypoint     models.TrackWaypoint
			content      sql.NullString
			mediaURLsRaw sql.NullString
		)
		if err := rows.Scan(
			&waypoint.ID, &waypoint.TrackID, &waypoint.UserID, &waypoint.Lat, &waypoint.Lng, &waypoint.Elevation,
			&waypoint.NodeTime, &waypoint.MediaType, &content, &mediaURLsRaw, &waypoint.CreatedAt,
		); err != nil {
			return nil, err
		}
		if content.Valid {
			waypoint.Content = content.String
		}
		if mediaURLsRaw.Valid && mediaURLsRaw.String != "" {
			if err := json.Unmarshal([]byte(mediaURLsRaw.String), &waypoint.MediaURLs); err != nil {
				return nil, err
			}
		}
		res = append(res, &waypoint)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return res, nil
}

// MySQLUserRepository implements UserRepository on top of MySQL.
type MySQLUserRepository struct{ db *sql.DB }

func NewMySQLUserRepository(db *sql.DB) *MySQLUserRepository { return &MySQLUserRepository{db: db} }

func (r *MySQLUserRepository) CreateIfNotExists(ctx context.Context, u *models.User) (*models.User, error) {
	// Fast path: if already exists, return it.
	if existing, err := r.FindByID(ctx, u.ID); err == nil {
		return existing, nil
	} else if !errors.Is(err, ErrNotFound) {
		return nil, err
	}

	now := time.Now()
	if u.CreatedAt.IsZero() {
		u.CreatedAt = now
	}
	u.UpdatedAt = u.CreatedAt

	if u.TokenVersion <= 0 {
		u.TokenVersion = 1
	}
	_, err := r.db.ExecContext(ctx,
		`INSERT INTO users (id, nickname, avatar_url, signature, phone, client_language, token_version, created_at, updated_at)
			 VALUES (?,?,?,?,?,?,?,?,?)
			 ON DUPLICATE KEY UPDATE id=id`,
		u.ID, u.Nickname, nullableStringValue(u.AvatarURL), nullableStringValue(u.Signature), u.Phone, u.ClientLanguage, u.TokenVersion, u.CreatedAt, u.UpdatedAt,
	)
	if err != nil {
		return nil, err
	}
	return r.FindByID(ctx, u.ID)
}

func (r *MySQLUserRepository) FindByID(ctx context.Context, id int64) (*models.User, error) {
	row := r.db.QueryRowContext(ctx,
		`SELECT id, nickname, avatar_url, signature, phone, client_language, token_version, created_at, updated_at FROM users WHERE id=?`,
		id,
	)
	var (
		u         models.User
		avatarURL sql.NullString
		signature sql.NullString
	)
	if err := row.Scan(&u.ID, &u.Nickname, &avatarURL, &signature, &u.Phone, &u.ClientLanguage, &u.TokenVersion, &u.CreatedAt, &u.UpdatedAt); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, err
	}
	if avatarURL.Valid {
		u.AvatarURL = avatarURL.String
	}
	if signature.Valid {
		u.Signature = signature.String
	}
	if u.TokenVersion <= 0 {
		u.TokenVersion = 1
	}
	return &u, nil
}

func (r *MySQLUserRepository) FindByPhone(ctx context.Context, phone string) (*models.User, error) {
	row := r.db.QueryRowContext(ctx,
		`SELECT id, nickname, avatar_url, signature, phone, client_language, token_version, created_at, updated_at FROM users WHERE phone=? LIMIT 1`,
		phone,
	)
	var (
		u         models.User
		avatarURL sql.NullString
		signature sql.NullString
	)
	if err := row.Scan(&u.ID, &u.Nickname, &avatarURL, &signature, &u.Phone, &u.ClientLanguage, &u.TokenVersion, &u.CreatedAt, &u.UpdatedAt); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, err
	}
	if avatarURL.Valid {
		u.AvatarURL = avatarURL.String
	}
	if signature.Valid {
		u.Signature = signature.String
	}
	if u.TokenVersion <= 0 {
		u.TokenVersion = 1
	}
	return &u, nil
}

func (r *MySQLUserRepository) FindByNickname(ctx context.Context, nickname string) (*models.User, error) {
	row := r.db.QueryRowContext(ctx,
		`SELECT id, nickname, avatar_url, signature, phone, client_language, token_version, created_at, updated_at FROM users WHERE nickname=? LIMIT 1`,
		nickname,
	)
	var (
		u         models.User
		avatarURL sql.NullString
		signature sql.NullString
	)
	if err := row.Scan(&u.ID, &u.Nickname, &avatarURL, &signature, &u.Phone, &u.ClientLanguage, &u.TokenVersion, &u.CreatedAt, &u.UpdatedAt); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, err
	}
	if avatarURL.Valid {
		u.AvatarURL = avatarURL.String
	}
	if signature.Valid {
		u.Signature = signature.String
	}
	if u.TokenVersion <= 0 {
		u.TokenVersion = 1
	}
	return &u, nil
}

func (r *MySQLUserRepository) Update(ctx context.Context, u *models.User) error {
	if u.TokenVersion <= 0 {
		u.TokenVersion = 1
	}
	u.UpdatedAt = time.Now()
	res, err := r.db.ExecContext(ctx,
		`UPDATE users SET nickname=?, avatar_url=?, signature=?, phone=?, client_language=?, token_version=?, updated_at=? WHERE id=?`,
		u.Nickname, nullableStringValue(u.AvatarURL), nullableStringValue(u.Signature), u.Phone, u.ClientLanguage, u.TokenVersion, u.UpdatedAt, u.ID,
	)
	if err != nil {
		return err
	}
	rows, _ := res.RowsAffected()
	if rows == 0 {
		return ErrNotFound
	}
	return nil
}

// ListAll 返回全量用户列表，按 created_at desc, id desc 排序。
func (r *MySQLUserRepository) ListAll(ctx context.Context, cursor *models.UserListCursor, limit int) ([]*models.User, error) {
	if limit <= 0 {
		limit = 20
	}
	query := `SELECT id, nickname, avatar_url, signature, phone, client_language, token_version, created_at, updated_at FROM users`
	args := make([]interface{}, 0, 4)
	if cursor != nil && !cursor.CreatedAt.IsZero() {
		query += ` WHERE (created_at < ? OR (created_at = ? AND id < ?))`
		args = append(args, cursor.CreatedAt, cursor.CreatedAt, cursor.ID)
	}
	query += ` ORDER BY created_at DESC, id DESC LIMIT ?`
	args = append(args, limit)
	rows, err := r.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	users := make([]*models.User, 0, limit)
	for rows.Next() {
		var (
			u         models.User
			avatarURL sql.NullString
			signature sql.NullString
		)
		if err := rows.Scan(&u.ID, &u.Nickname, &avatarURL, &signature, &u.Phone, &u.ClientLanguage, &u.TokenVersion, &u.CreatedAt, &u.UpdatedAt); err != nil {
			return nil, err
		}
		if avatarURL.Valid {
			u.AvatarURL = avatarURL.String
		}
		if signature.Valid {
			u.Signature = signature.String
		}
		if u.TokenVersion <= 0 {
			u.TokenVersion = 1
		}
		users = append(users, &u)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return users, nil
}

// CountAll 返回全量用户总数。
func (r *MySQLUserRepository) CountAll(ctx context.Context) (int64, error) {
	row := r.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM users`)
	var count int64
	if err := row.Scan(&count); err != nil {
		return 0, err
	}
	return count, nil
}

// MySQLCollectRepository implements CollectRepository on top of MySQL.
type MySQLCollectRepository struct{ db *sql.DB }

func NewMySQLCollectRepository(db *sql.DB) *MySQLCollectRepository {
	return &MySQLCollectRepository{db: db}
}

func (r *MySQLCollectRepository) IsCollected(ctx context.Context, userID int64, trackID string) (bool, error) {
	row := r.db.QueryRowContext(ctx,
		`SELECT 1 FROM track_collects WHERE user_id=? AND track_id=? LIMIT 1`,
		userID, trackID,
	)
	var one int
	if err := row.Scan(&one); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return false, nil
		}
		return false, err
	}
	return true, nil
}

func (r *MySQLCollectRepository) ListByUserID(ctx context.Context, userID int64, cursor *models.TrackCollectCursor, limit int) ([]*models.TrackCollect, error) {
	if userID <= 0 {
		return nil, nil
	}
	if limit <= 0 {
		return []*models.TrackCollect{}, nil
	}

	// Order: created_at desc, track_id desc
	query := `SELECT user_id, track_id, created_at FROM track_collects WHERE user_id=?`
	args := make([]any, 0, 4)
	args = append(args, userID)
	if cursor != nil && !cursor.CreatedAt.IsZero() && cursor.TrackID != "" {
		query += ` AND (created_at < ? OR (created_at = ? AND track_id < ?))`
		args = append(args, cursor.CreatedAt, cursor.CreatedAt, cursor.TrackID)
	}
	query += ` ORDER BY created_at DESC, track_id DESC LIMIT ?`
	args = append(args, limit)

	rows, err := r.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	res := make([]*models.TrackCollect, 0, limit)
	for rows.Next() {
		var c models.TrackCollect
		if err := rows.Scan(&c.UserID, &c.TrackID, &c.CreatedAt); err != nil {
			return nil, err
		}
		res = append(res, &c)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return res, nil
}

// CountVisibleByUserID returns the count of collected tracks that are visible in collected list.
// Visibility rule keeps consistent with TrackService.ListCollectedTracks:
// - track exists
// - track.status == Normal
// - track.is_running == 0
func (r *MySQLCollectRepository) CountVisibleByUserID(ctx context.Context, userID int64) (int64, error) {
	if userID <= 0 {
		return 0, nil
	}
	row := r.db.QueryRowContext(ctx,
		`SELECT COUNT(*)
		 FROM track_collects c
		 JOIN track_records t ON c.track_id = t.id
		 WHERE c.user_id=? AND t.status=? AND t.is_running=0`,
		userID, models.TrackStatusNormal,
	)
	var cnt int64
	if err := row.Scan(&cnt); err != nil {
		return 0, err
	}
	return cnt, nil
}

func (r *MySQLCollectRepository) RemoveByTrackID(ctx context.Context, trackID string) error {
	if trackID == "" {
		return nil
	}
	_, err := r.db.ExecContext(ctx, `DELETE FROM track_collects WHERE track_id=?`, trackID)
	return err
}

func (r *MySQLCollectRepository) CountByTrackIDs(ctx context.Context, trackIDs []string) (map[string]int64, error) {
	res := make(map[string]int64, len(trackIDs))
	if len(trackIDs) == 0 {
		return res, nil
	}

	// 1) 先对入参去重并过滤空串：
	// - 保护后续 IN (...) 的 SQL 长度与参数数量
	// - 同时确保返回 map 至少包含调用方关心的 key（未命中时为 0）
	uniq := make(map[string]struct{}, len(trackIDs))
	uniqIDs := make([]string, 0, len(trackIDs))
	for _, id := range trackIDs {
		if id == "" {
			continue
		}
		if _, ok := uniq[id]; ok {
			continue
		}
		uniq[id] = struct{}{}
		uniqIDs = append(uniqIDs, id)
		res[id] = 0
	}
	if len(uniqIDs) == 0 {
		return res, nil
	}

	// 2) 用 GROUP BY 统计收藏总数。
	// track_collects 表对 (user_id, track_id) 具备唯一约束（见 AddCollect 的 ON DUPLICATE KEY），
	// 因此 COUNT(*) 的语义是“收藏该轨迹的用户数”。
	placeholders := strings.TrimRight(strings.Repeat("?,", len(uniqIDs)), ",")
	query := fmt.Sprintf(
		`SELECT track_id, COUNT(*) AS cnt FROM track_collects WHERE track_id IN (%s) GROUP BY track_id`,
		placeholders,
	)
	args := make([]any, 0, len(uniqIDs))
	for _, id := range uniqIDs {
		args = append(args, id)
	}
	rows, err := r.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	for rows.Next() {
		var (
			trackID string
			cnt     int64
		)
		if err := rows.Scan(&trackID, &cnt); err != nil {
			return nil, err
		}
		res[trackID] = cnt
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return res, nil
}

func (r *MySQLCollectRepository) AddCollect(ctx context.Context, userID int64, trackID string) error {
	_, err := r.db.ExecContext(ctx,
		`INSERT INTO track_collects (user_id, track_id, created_at)
		 VALUES (?,?,?)
		 ON DUPLICATE KEY UPDATE created_at=created_at`,
		userID, trackID, time.Now(),
	)
	if err != nil {
		return err
	}
	return nil
}

func (r *MySQLCollectRepository) RemoveCollect(ctx context.Context, userID int64, trackID string) error {
	_, err := r.db.ExecContext(ctx, `DELETE FROM track_collects WHERE user_id=? AND track_id=?`, userID, trackID)
	if err != nil {
		return err
	}
	return nil
}

// MySQLFollowRepository implements FollowRepository on top of MySQL.
type MySQLFollowRepository struct{ db *sql.DB }

func NewMySQLFollowRepository(db *sql.DB) *MySQLFollowRepository {
	return &MySQLFollowRepository{db: db}
}

func (r *MySQLFollowRepository) IsFollowing(ctx context.Context, followerUserID int64, followeeUserID int64) (bool, error) {
	row := r.db.QueryRowContext(ctx,
		`SELECT 1 FROM user_follows WHERE follower_user_id=? AND followee_user_id=? LIMIT 1`,
		followerUserID, followeeUserID,
	)
	var one int
	if err := row.Scan(&one); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return false, nil
		}
		return false, err
	}
	return true, nil
}

func (r *MySQLFollowRepository) AddFollow(ctx context.Context, followerUserID int64, followeeUserID int64) error {
	_, err := r.db.ExecContext(ctx,
		`INSERT INTO user_follows (follower_user_id, followee_user_id, created_at)
		 VALUES (?,?,?)
		 ON DUPLICATE KEY UPDATE created_at=created_at`,
		followerUserID, followeeUserID, time.Now(),
	)
	return err
}

func (r *MySQLFollowRepository) RemoveFollow(ctx context.Context, followerUserID int64, followeeUserID int64) error {
	_, err := r.db.ExecContext(ctx, `DELETE FROM user_follows WHERE follower_user_id=? AND followee_user_id=?`, followerUserID, followeeUserID)
	return err
}

func (r *MySQLFollowRepository) ListFollowing(ctx context.Context, userID int64, cursor *models.UserFollowCursor, limit int) ([]*models.UserFollow, error) {
	if userID <= 0 || limit <= 0 {
		return []*models.UserFollow{}, nil
	}
	query := `SELECT follower_user_id, followee_user_id, created_at FROM user_follows WHERE follower_user_id=?`
	args := []any{userID}
	if cursor != nil && !cursor.CreatedAt.IsZero() && cursor.UserID > 0 {
		query += ` AND (created_at < ? OR (created_at = ? AND followee_user_id < ?))`
		args = append(args, cursor.CreatedAt, cursor.CreatedAt, cursor.UserID)
	}
	query += ` ORDER BY created_at DESC, followee_user_id DESC LIMIT ?`
	args = append(args, limit)
	return r.listFollows(ctx, query, args, limit)
}

func (r *MySQLFollowRepository) ListFollowers(ctx context.Context, userID int64, cursor *models.UserFollowCursor, limit int) ([]*models.UserFollow, error) {
	if userID <= 0 || limit <= 0 {
		return []*models.UserFollow{}, nil
	}
	query := `SELECT follower_user_id, followee_user_id, created_at FROM user_follows WHERE followee_user_id=?`
	args := []any{userID}
	if cursor != nil && !cursor.CreatedAt.IsZero() && cursor.UserID > 0 {
		query += ` AND (created_at < ? OR (created_at = ? AND follower_user_id < ?))`
		args = append(args, cursor.CreatedAt, cursor.CreatedAt, cursor.UserID)
	}
	query += ` ORDER BY created_at DESC, follower_user_id DESC LIMIT ?`
	args = append(args, limit)
	return r.listFollows(ctx, query, args, limit)
}

func (r *MySQLFollowRepository) listFollows(ctx context.Context, query string, args []any, limit int) ([]*models.UserFollow, error) {
	rows, err := r.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	res := make([]*models.UserFollow, 0, limit)
	for rows.Next() {
		var f models.UserFollow
		if err := rows.Scan(&f.FollowerUserID, &f.FolloweeUserID, &f.CreatedAt); err != nil {
			return nil, err
		}
		res = append(res, &f)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return res, nil
}

func (r *MySQLFollowRepository) CountFollowing(ctx context.Context, userID int64) (int64, error) {
	return r.countFollows(ctx, `SELECT COUNT(*) FROM user_follows WHERE follower_user_id=?`, userID)
}

func (r *MySQLFollowRepository) CountFollowers(ctx context.Context, userID int64) (int64, error) {
	return r.countFollows(ctx, `SELECT COUNT(*) FROM user_follows WHERE followee_user_id=?`, userID)
}

func (r *MySQLFollowRepository) countFollows(ctx context.Context, query string, userID int64) (int64, error) {
	if userID <= 0 {
		return 0, nil
	}
	row := r.db.QueryRowContext(ctx, query, userID)
	var cnt int64
	if err := row.Scan(&cnt); err != nil {
		return 0, err
	}
	return cnt, nil
}

// MySQLNavigationRepository implements NavigationRepository on top of MySQL.
type MySQLNavigationRepository struct{ db *sql.DB }

func NewMySQLNavigationRepository(db *sql.DB) *MySQLNavigationRepository {
	return &MySQLNavigationRepository{db: db}
}

func (r *MySQLNavigationRepository) AddNavigation(ctx context.Context, navigatorUserID int64, trackID string) error {
	if navigatorUserID <= 0 || trackID == "" {
		return nil
	}
	_, err := r.db.ExecContext(ctx,
		`INSERT INTO track_navigations (track_id, navigator_user_id, created_at) VALUES (?,?,?)`,
		trackID, navigatorUserID, time.Now(),
	)
	return err
}

func (r *MySQLNavigationRepository) CountByTrackIDs(ctx context.Context, trackIDs []string) (map[string]int64, error) {
	res := make(map[string]int64, len(trackIDs))
	if len(trackIDs) == 0 {
		return res, nil
	}

	uniq := make(map[string]struct{}, len(trackIDs))
	uniqIDs := make([]string, 0, len(trackIDs))
	for _, id := range trackIDs {
		if id == "" {
			continue
		}
		if _, ok := uniq[id]; ok {
			continue
		}
		uniq[id] = struct{}{}
		uniqIDs = append(uniqIDs, id)
		res[id] = 0
	}
	if len(uniqIDs) == 0 {
		return res, nil
	}

	placeholders := strings.TrimRight(strings.Repeat("?,", len(uniqIDs)), ",")
	query := fmt.Sprintf(
		`SELECT track_id, COUNT(*) AS cnt FROM track_navigations WHERE track_id IN (%s) GROUP BY track_id`,
		placeholders,
	)
	args := make([]any, 0, len(uniqIDs))
	for _, id := range uniqIDs {
		args = append(args, id)
	}
	rows, err := r.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	for rows.Next() {
		var (
			trackID string
			cnt     int64
		)
		if err := rows.Scan(&trackID, &cnt); err != nil {
			return nil, err
		}
		res[trackID] = cnt
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return res, nil
}

// CountByTrackOwnerUserID returns total navigation usage count for tracks owned by the user.
// 口径与 TrackRepository.ListByUserID 保持一致：排除删除与进行中轨迹。
func (r *MySQLNavigationRepository) CountByTrackOwnerUserID(ctx context.Context, ownerUserID int64) (int64, error) {
	row := r.db.QueryRowContext(
		ctx,
		`SELECT COUNT(*)
		 FROM track_navigations n
		 JOIN track_records t ON n.track_id = t.id
		 WHERE t.user_id=? AND t.is_running=0 AND t.status IN (?, ?)`,
		ownerUserID, models.TrackStatusNormal, models.TrackStatusPrivate,
	)
	var cnt int64
	if err := row.Scan(&cnt); err != nil {
		return 0, err
	}
	return cnt, nil
}

// MySQLLoginLogRepository implements LoginLogRepository on top of MySQL.
type MySQLLoginLogRepository struct{ db *sql.DB }

func NewMySQLLoginLogRepository(db *sql.DB) *MySQLLoginLogRepository {
	return &MySQLLoginLogRepository{db: db}
}

func (r *MySQLLoginLogRepository) Create(ctx context.Context, log *models.LoginLog) error {
	if log.CreatedAt.IsZero() {
		log.CreatedAt = time.Now()
	}
	res, err := r.db.ExecContext(ctx,
		`INSERT INTO login_log (user_id, login_type, ip, device_id, platform, created_at)
		 VALUES (?,?,?,?,?,?)`,
		log.UserID, log.LoginType, nullableStringValue(log.IP), nullableStringValue(log.DeviceID), nullableStringValue(log.Platform), log.CreatedAt,
	)
	if err != nil {
		return err
	}
	id, err := res.LastInsertId()
	if err != nil {
		return err
	}
	log.ID = id
	return nil
}

func (r *MySQLLoginLogRepository) ListByUserID(ctx context.Context, userID int64, limit int) ([]*models.LoginLog, error) {
	if limit <= 0 {
		limit = 20
	}
	rows, err := r.db.QueryContext(ctx,
		`SELECT id, user_id, login_type, ip, device_id, platform, created_at
		 FROM login_log WHERE user_id=? ORDER BY created_at DESC, id DESC LIMIT ?`,
		userID, limit,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	logs := make([]*models.LoginLog, 0, limit)
	for rows.Next() {
		var (
			item     models.LoginLog
			ip       sql.NullString
			deviceID sql.NullString
			platform sql.NullString
		)
		if err := rows.Scan(&item.ID, &item.UserID, &item.LoginType, &ip, &deviceID, &platform, &item.CreatedAt); err != nil {
			return nil, err
		}
		if ip.Valid {
			item.IP = ip.String
		}
		if deviceID.Valid {
			item.DeviceID = deviceID.String
		}
		if platform.Valid {
			item.Platform = platform.String
		}
		logs = append(logs, &item)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return logs, nil
}

// OpenMySQL opens a MySQL connection pool with some safe defaults.
func OpenMySQL(dsn string) (*sql.DB, error) {
	cfg, err := mysql.ParseDSN(dsn)
	if err != nil {
		return nil, err
	}
	// Ensure time parsing works across the codebase.
	if cfg.Params == nil {
		cfg.Params = map[string]string{}
	}
	if _, ok := cfg.Params["parseTime"]; !ok {
		cfg.Params["parseTime"] = "true"
	}
	if _, ok := cfg.Params["charset"]; !ok {
		cfg.Params["charset"] = "utf8mb4"
	}
	if _, ok := cfg.Params["collation"]; !ok {
		cfg.Params["collation"] = "utf8mb4_unicode_ci"
	}

	db, err := sql.Open("mysql", cfg.FormatDSN())
	if err != nil {
		return nil, err
	}
	// Conservative defaults; caller can tune if needed.
	db.SetMaxOpenConns(10)
	db.SetMaxIdleConns(5)
	db.SetConnMaxLifetime(30 * time.Minute)
	return db, nil
}

// MySQLAppReleaseRepository implements AppReleaseRepository on top of MySQL.
type MySQLAppReleaseRepository struct{ db *sql.DB }

func NewMySQLAppReleaseRepository(db *sql.DB) *MySQLAppReleaseRepository {
	return &MySQLAppReleaseRepository{db: db}
}

// Upsert 插入或更新一条发布记录。
//
// 通过 (platform, version_code) 的唯一键实现幂等：
// - 首次插入时填充所有字段；
// - 再次插入同一 (platform, version_code) 时，更新可变字段（保留 created_at）。
func (r *MySQLAppReleaseRepository) Upsert(ctx context.Context, release *models.AppRelease) error {
	if release == nil {
		return errors.New("release is nil")
	}
	if release.Platform == "" {
		return errors.New("release.platform is required")
	}
	if release.VersionCode <= 0 {
		return errors.New("release.version_code must be > 0")
	}
	now := time.Now()
	if release.CreatedAt.IsZero() {
		release.CreatedAt = now
	}
	release.UpdatedAt = now
	if release.Status == "" {
		release.Status = models.AppReleaseStatusPublished
	}

	res, err := r.db.ExecContext(ctx,
		`INSERT INTO app_releases (
			platform, version_name, version_code, min_supported_version_code,
			package_url, package_size, package_md5, release_notes, force_update,
			status, operator_user_id, operator_name, created_at, updated_at
		) VALUES (?,?,?,?,?,?,?,?,?,?,?,?,?,?)
		ON DUPLICATE KEY UPDATE
			version_name=VALUES(version_name),
			min_supported_version_code=VALUES(min_supported_version_code),
			package_url=VALUES(package_url),
			package_size=VALUES(package_size),
			package_md5=VALUES(package_md5),
			release_notes=VALUES(release_notes),
			force_update=VALUES(force_update),
			status=VALUES(status),
			operator_user_id=VALUES(operator_user_id),
			operator_name=VALUES(operator_name),
			updated_at=VALUES(updated_at)`,
		release.Platform, release.VersionName, release.VersionCode, release.MinSupportedVersionCode,
		release.PackageURL, release.PackageSize, release.PackageMD5, nullableStringValue(release.ReleaseNotes), boolToInt(release.ForceUpdate),
		release.Status, release.OperatorUserID, release.OperatorName, release.CreatedAt, release.UpdatedAt,
	)
	if err != nil {
		return err
	}
	// LastInsertId：插入时返回新自增 id；更新时 MySQL 返回 0。
	id, _ := res.LastInsertId()
	if id > 0 {
		release.ID = id
		return nil
	}
	// 更新分支：回查一次拿到实际 id 及 created_at。
	existing, err := r.GetByPlatformVersion(ctx, release.Platform, release.VersionCode)
	if err != nil {
		return err
	}
	release.ID = existing.ID
	release.CreatedAt = existing.CreatedAt
	return nil
}

func (r *MySQLAppReleaseRepository) GetByID(ctx context.Context, id int64) (*models.AppRelease, error) {
	row := r.db.QueryRowContext(ctx, appReleaseSelectSQL()+` WHERE id=?`, id)
	return scanAppReleaseRow(row)
}

func (r *MySQLAppReleaseRepository) GetByPlatformVersion(ctx context.Context, platform models.AppReleasePlatform, versionCode int64) (*models.AppRelease, error) {
	row := r.db.QueryRowContext(ctx, appReleaseSelectSQL()+` WHERE platform=? AND version_code=?`, platform, versionCode)
	return scanAppReleaseRow(row)
}

func (r *MySQLAppReleaseRepository) List(ctx context.Context, filter models.AppReleaseListFilter) ([]*models.AppRelease, error) {
	query := appReleaseSelectSQL() + ` WHERE 1=1`
	args := make([]any, 0, 2)
	if filter.Platform != "" {
		query += ` AND platform=?`
		args = append(args, filter.Platform)
	}
	if filter.Status != "" {
		query += ` AND status=?`
		args = append(args, filter.Status)
	}
	query += ` ORDER BY platform ASC, version_code DESC`
	rows, err := r.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	res := make([]*models.AppRelease, 0)
	for rows.Next() {
		item, err := scanAppRelease(rows)
		if err != nil {
			return nil, err
		}
		res = append(res, item)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return res, nil
}

func (r *MySQLAppReleaseRepository) GetLatestPublished(ctx context.Context, platform models.AppReleasePlatform) (*models.AppRelease, error) {
	row := r.db.QueryRowContext(ctx,
		appReleaseSelectSQL()+` WHERE platform=? AND status=? ORDER BY version_code DESC LIMIT 1`,
		platform, models.AppReleaseStatusPublished,
	)
	return scanAppReleaseRow(row)
}

func (r *MySQLAppReleaseRepository) Delete(ctx context.Context, id int64) error {
	res, err := r.db.ExecContext(ctx, `DELETE FROM app_releases WHERE id=?`, id)
	if err != nil {
		return err
	}
	rows, _ := res.RowsAffected()
	if rows == 0 {
		return ErrNotFound
	}
	return nil
}

// MySQLFeedbackRepository implements FeedbackRepository on top of MySQL.
type MySQLFeedbackRepository struct{ db *sql.DB }

func NewMySQLFeedbackRepository(db *sql.DB) *MySQLFeedbackRepository {
	return &MySQLFeedbackRepository{db: db}
}

func (r *MySQLFeedbackRepository) Create(ctx context.Context, feedback *models.Feedback) error {
	if feedback == nil {
		return errors.New("feedback is nil")
	}
	imagesJSON, err := json.Marshal(feedback.Images)
	if err != nil {
		return err
	}
	now := time.Now()
	if feedback.CreatedAt.IsZero() {
		feedback.CreatedAt = now
	}
	feedback.UpdatedAt = feedback.CreatedAt
	if feedback.Status == "" {
		feedback.Status = models.FeedbackStatusPending
	}
	res, err := r.db.ExecContext(ctx, `INSERT INTO user_feedbacks (
		feedback_id, user_id, content, images_json, contact, app_version, platform,
		device_model, system_version, status, reply, created_at, updated_at
	) VALUES (?,?,?,?,?,?,?,?,?,?,?,?,?)`,
		feedback.FeedbackID, feedback.UserID, feedback.Content, string(imagesJSON), feedback.Contact,
		feedback.AppVersion, feedback.Platform, feedback.DeviceModel, feedback.SystemVersion,
		feedback.Status, nullableStringValue(feedback.Reply), feedback.CreatedAt, feedback.UpdatedAt,
	)
	if err != nil {
		var mysqlErr *mysql.MySQLError
		if errors.As(err, &mysqlErr) && mysqlErr.Number == 1062 {
			return ErrAlreadyExists
		}
		return err
	}
	id, _ := res.LastInsertId()
	feedback.ID = id
	return nil
}

func (r *MySQLFeedbackRepository) FindByFeedbackID(ctx context.Context, feedbackID string) (*models.Feedback, error) {
	row := r.db.QueryRowContext(ctx, feedbackSelectSQL()+` WHERE feedback_id=?`, feedbackID)
	return scanFeedbackRow(row)
}

func (r *MySQLFeedbackRepository) List(ctx context.Context, filter models.FeedbackListFilter) ([]*models.Feedback, error) {
	limit := filter.Limit
	if limit <= 0 {
		limit = 20
	}
	if limit > 100 {
		limit = 100
	}
	query := feedbackSelectSQL() + ` WHERE 1=1`
	args := make([]any, 0, 5)
	if filter.UserID > 0 {
		query += ` AND user_id=?`
		args = append(args, filter.UserID)
	}
	if filter.Status != "" {
		query += ` AND status=?`
		args = append(args, filter.Status)
	}
	if filter.Cursor != nil && !filter.Cursor.CreatedAt.IsZero() && filter.Cursor.FeedbackID != "" {
		query += ` AND (created_at < ? OR (created_at = ? AND feedback_id < ?))`
		args = append(args, filter.Cursor.CreatedAt, filter.Cursor.CreatedAt, filter.Cursor.FeedbackID)
	}
	query += ` ORDER BY created_at DESC, feedback_id DESC LIMIT ?`
	args = append(args, limit)

	rows, err := r.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	res := make([]*models.Feedback, 0, limit)
	for rows.Next() {
		item, err := scanFeedback(rows)
		if err != nil {
			return nil, err
		}
		res = append(res, item)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return res, nil
}

func (r *MySQLFeedbackRepository) CountByUserAndStatuses(ctx context.Context, userID int64, statuses []models.FeedbackStatus) (int64, error) {
	if userID <= 0 || len(statuses) == 0 {
		return 0, nil
	}
	placeholders := make([]string, 0, len(statuses))
	args := make([]any, 0, len(statuses)+1)
	args = append(args, userID)
	for _, status := range statuses {
		placeholders = append(placeholders, "?")
		args = append(args, status)
	}
	query := fmt.Sprintf(`SELECT COUNT(*) FROM user_feedbacks WHERE user_id=? AND status IN (%s)`, strings.Join(placeholders, ","))
	var count int64
	if err := r.db.QueryRowContext(ctx, query, args...).Scan(&count); err != nil {
		return 0, err
	}
	return count, nil
}

func (r *MySQLFeedbackRepository) UpdateStatus(ctx context.Context, feedbackID string, status models.FeedbackStatus, reply string) error {
	res, err := r.db.ExecContext(ctx, `UPDATE user_feedbacks SET status=?, reply=?, updated_at=? WHERE feedback_id=?`,
		status, nullableStringValue(reply), time.Now(), feedbackID,
	)
	if err != nil {
		return err
	}
	rows, _ := res.RowsAffected()
	if rows == 0 {
		return ErrNotFound
	}
	return nil
}

func appReleaseSelectSQL() string {
	return `SELECT id, platform, version_name, version_code, min_supported_version_code,
		package_url, package_size, package_md5, release_notes, force_update,
		status, operator_user_id, operator_name, created_at, updated_at
	FROM app_releases`
}

func feedbackSelectSQL() string {
	return `SELECT id, feedback_id, user_id, content, images_json, contact, app_version, platform,
		device_model, system_version, status, reply, created_at, updated_at
	FROM user_feedbacks`
}

type feedbackRowScanner interface {
	Scan(dest ...any) error
}

func scanFeedback(row feedbackRowScanner) (*models.Feedback, error) {
	var (
		item       models.Feedback
		imagesJSON sql.NullString
		reply      sql.NullString
	)
	if err := row.Scan(
		&item.ID, &item.FeedbackID, &item.UserID, &item.Content, &imagesJSON, &item.Contact,
		&item.AppVersion, &item.Platform, &item.DeviceModel, &item.SystemVersion,
		&item.Status, &reply, &item.CreatedAt, &item.UpdatedAt,
	); err != nil {
		return nil, err
	}
	if imagesJSON.Valid && imagesJSON.String != "" {
		if err := json.Unmarshal([]byte(imagesJSON.String), &item.Images); err != nil {
			return nil, err
		}
	}
	if reply.Valid {
		item.Reply = reply.String
	}
	return &item, nil
}

func scanFeedbackRow(row *sql.Row) (*models.Feedback, error) {
	item, err := scanFeedback(row)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, err
	}
	return item, nil
}

// appReleaseRowScanner 抽象 sql.Row 与 sql.Rows 的 Scan 方法，便于统一扫描逻辑。
type appReleaseRowScanner interface {
	Scan(dest ...any) error
}

func scanAppRelease(row appReleaseRowScanner) (*models.AppRelease, error) {
	var (
		item         models.AppRelease
		releaseNotes sql.NullString
		forceUpdate  int
	)
	if err := row.Scan(
		&item.ID, &item.Platform, &item.VersionName, &item.VersionCode, &item.MinSupportedVersionCode,
		&item.PackageURL, &item.PackageSize, &item.PackageMD5, &releaseNotes, &forceUpdate,
		&item.Status, &item.OperatorUserID, &item.OperatorName, &item.CreatedAt, &item.UpdatedAt,
	); err != nil {
		return nil, err
	}
	if releaseNotes.Valid {
		item.ReleaseNotes = releaseNotes.String
	}
	item.ForceUpdate = forceUpdate != 0
	return &item, nil
}

func scanAppReleaseRow(row *sql.Row) (*models.AppRelease, error) {
	item, err := scanAppRelease(row)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, err
	}
	return item, nil
}

func boolToInt(b bool) int {
	if b {
		return 1
	}
	return 0
}
