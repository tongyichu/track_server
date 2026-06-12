CREATE TABLE `users` (
                          `id` BIGINT NOT NULL,
                          `nickname` varchar(255) NOT NULL DEFAULT '' COMMENT '用户昵称',
                          `avatar_url` varchar(255) COMMENT '用户头像URI',
                          `signature` text,
                          `phone` varchar(32) NOT NULL DEFAULT '' COMMENT '用户手机号',
                          `client_language` varchar(64) NOT NULL DEFAULT '' COMMENT '客户端语言',
                          `token_version` bigint NOT NULL DEFAULT '1' COMMENT '登录令牌版本号',
                          `created_at` datetime(6) NOT NULL,
                          `updated_at` datetime(6) NOT NULL,
                          PRIMARY KEY (`id`),
                          KEY `idx_users_updated_at` (`updated_at`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COMMENT='用户基础资料表';


CREATE TABLE `track_records` (
                                 `id` VARCHAR(64) NOT NULL,
                                 `user_id` bigint unsigned NOT NULL COMMENT '用户ID',
                                 `session_id` varchar(64) NOT NULL DEFAULT '' COMMENT '关联的同行会话ID',
                                 `city_code` varchar(16) NOT NULL DEFAULT '' COMMENT '城市Code',
                                 `locate_addr` varchar(255) NOT NULL DEFAULT '' COMMENT '轨迹的具体位置信息',
                                `track_type` varchar(32) NOT NULL DEFAULT '' COMMENT '轨迹类型，如徒步/跑步/骑车/自驾',
                                `source_tag` varchar(64) NOT NULL DEFAULT '' COMMENT '轨迹来源/运营标签',
                                `coordinate_system` varchar(32) NOT NULL DEFAULT '' COMMENT '坐标系',
                                 `title` varchar(128) NOT NULL DEFAULT '' COMMENT '轨迹名称',
                                 `start_time` datetime NOT NULL COMMENT '开始时间',
                                 `end_time` datetime COMMENT '结束时间',
                                 `distance` decimal(10,2) DEFAULT '0.00' COMMENT '总距离(米)',
                                 `duration` int unsigned DEFAULT '0' COMMENT '运动耗时(秒)',
                                 `calories_burned` decimal(10,2) DEFAULT '0.00' COMMENT '热量消耗(千卡)',
                                 `elevation_gain` int DEFAULT '0' COMMENT '累计爬升(米)',
                                 `raw_track_url` varchar(255) COMMENT '指向对象存储中原始轨迹点文件(JSON/GeoJSON)的URL',
                                 `track_screenshot_url` varchar(255) COMMENT '轨迹截图文件在对象存储中的地址',
                                 `track_no_map_bg_screenshot_url` varchar(255) COMMENT '没有地图背景的轨迹路线截图URI',
                                 `is_running` tinyint NOT NULL DEFAULT '1' COMMENT '表示轨迹是否仍处于进行中: 0-未运行, 1-运行中',
                                 `status` tinyint NOT NULL DEFAULT '1' COMMENT '状态: 0-删除, 1-正常, 2-私密',
                                 `avg_speed_kmh` double NOT NULL DEFAULT '1' COMMENT '平均速度，单位 km/h',
                                 `created_at` timestamp NOT NULL DEFAULT CURRENT_TIMESTAMP,
							 `updated_at` timestamp NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
                                 `deleted_at` datetime DEFAULT NULL COMMENT '删除时间',
                                 PRIMARY KEY (`id`),
                                 KEY `idx_track_session` (`session_id`),
                                 KEY `idx_track_source_tag` (`source_tag`),
                                 KEY `idx_user_time` (`user_id`,`start_time`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COMMENT='轨迹概要信息表';


CREATE TABLE `track_id_sequences` (
                                     `id` BIGINT UNSIGNED NOT NULL AUTO_INCREMENT,
                                     PRIMARY KEY (`id`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COMMENT='轨迹ID全局序列表';


CREATE TABLE `track_waypoints` (
                                   `id` bigint unsigned NOT NULL AUTO_INCREMENT,
                                   `track_id` varchar(128) NOT NULL COMMENT '关联的轨迹主表ID',
                                   `user_id` bigint unsigned NOT NULL COMMENT '用户ID(冗余字段方便权限控制)',
                                   `lat` decimal(10,7) NOT NULL COMMENT '纬度',
                                   `lng` decimal(10,7) NOT NULL COMMENT '经度',
                                   `elevation` int NOT NULL DEFAULT '0' COMMENT '海拔',
                                   `node_time` datetime NOT NULL COMMENT '该节点的时间戳',
                                   `media_type` tinyint NOT NULL COMMENT '媒体类型: 1-纯文本, 2-图片, 3-语音, 4-图文+语音',
                                   `content` text COMMENT '文字描述',
                                   `media_urls` json COMMENT '媒体文件地址数组(如包含多张图片或语音的CDN URL)',
                                   `created_at` timestamp NOT NULL DEFAULT CURRENT_TIMESTAMP,
                                   PRIMARY KEY (`id`),
                                   KEY `idx_track_id` (`track_id`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COMMENT='轨迹媒体节点/航点表';


CREATE TABLE `track_collects` (
                                  `user_id` BIGINT NOT NULL COMMENT '收藏用户ID',
                                  `track_id` VARCHAR(64) NOT NULL COMMENT '轨迹ID',
                                  `created_at` DATETIME(6) NOT NULL DEFAULT CURRENT_TIMESTAMP(6) COMMENT '收藏时间',
                                  PRIMARY KEY (`user_id`, `track_id`),
                                  KEY `idx_collects_track` (`track_id`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COMMENT='用户收藏轨迹关系表';


CREATE TABLE `user_follows` (
                                `follower_user_id` BIGINT NOT NULL COMMENT '关注者用户ID',
                                `followee_user_id` BIGINT NOT NULL COMMENT '被关注者用户ID',
                                `created_at` DATETIME(6) NOT NULL DEFAULT CURRENT_TIMESTAMP(6) COMMENT '关注时间',
                                PRIMARY KEY (`follower_user_id`, `followee_user_id`),
                                KEY `idx_user_follows_followee` (`followee_user_id`, `created_at`, `follower_user_id`),
                                KEY `idx_user_follows_follower` (`follower_user_id`, `created_at`, `followee_user_id`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COMMENT='用户关注关系表';


CREATE TABLE `track_navigations` (
                                     `id` BIGINT UNSIGNED NOT NULL AUTO_INCREMENT,
                                     `track_id` VARCHAR(64) NOT NULL COMMENT '轨迹ID',
                                     `navigator_user_id` BIGINT NOT NULL COMMENT '导航使用者用户ID',
                                     `created_at` DATETIME(6) NOT NULL DEFAULT CURRENT_TIMESTAMP(6) COMMENT '使用时间',
                                     PRIMARY KEY (`id`),
                                     KEY `idx_nav_track` (`track_id`),
                                     KEY `idx_nav_user` (`navigator_user_id`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COMMENT='轨迹导航使用记录表';


CREATE TABLE `user_achievement_rewards` (
                                            `id` BIGINT UNSIGNED NOT NULL AUTO_INCREMENT,
                                            `user_id` BIGINT NOT NULL COMMENT '用户ID',
                                            `reward_code` VARCHAR(64) NOT NULL COMMENT '成就奖励编码',
                                            `source_track_id` VARCHAR(64) NOT NULL DEFAULT '' COMMENT '触发轨迹ID',
                                            `source_session_id` VARCHAR(64) NOT NULL DEFAULT '' COMMENT '触发同行会话ID',
                                            `progress_snapshot` TEXT COMMENT '发放时进度快照',
                                            `earned_at` DATETIME(6) NOT NULL COMMENT '获得时间',
                                            PRIMARY KEY (`id`),
                                            UNIQUE KEY `uk_user_reward` (`user_id`, `reward_code`),
                                            KEY `idx_user_achievement_earned` (`user_id`, `earned_at`, `id`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COMMENT='用户成就奖励记录表';


CREATE TABLE `app_releases` (
                                `id` BIGINT UNSIGNED NOT NULL AUTO_INCREMENT,
                                `platform` VARCHAR(16) NOT NULL COMMENT 'android / ios',
                                `version_name` VARCHAR(32) NOT NULL COMMENT '版本名，例如 1.2.3',
                                `version_code` INT UNSIGNED NOT NULL COMMENT '版本号，单调递增整数',
                                `min_supported_version_code` INT UNSIGNED NOT NULL DEFAULT '0' COMMENT '低于该版本必须强升',
                                `package_url` VARCHAR(512) NOT NULL DEFAULT '' COMMENT '安装包下载地址（iOS 通常为 AppStore 跳转链接）',
                                `package_size` BIGINT UNSIGNED NOT NULL DEFAULT '0' COMMENT '安装包大小（字节）',
                                `package_md5` VARCHAR(64) NOT NULL DEFAULT '' COMMENT '安装包 md5，可选',
                                `release_notes` TEXT COMMENT '版本说明',
                                `force_update` TINYINT NOT NULL DEFAULT '0' COMMENT '是否强制升级',
                                `status` VARCHAR(16) NOT NULL DEFAULT 'published' COMMENT 'draft / published / archived',
                                `operator_user_id` BIGINT NOT NULL DEFAULT '0' COMMENT '发布操作者',
                                `operator_name` VARCHAR(64) NOT NULL DEFAULT '' COMMENT '发布操作者名称',
                                `created_at` TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
                                `updated_at` TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
                                PRIMARY KEY (`id`),
                                UNIQUE KEY `uk_platform_version_code` (`platform`,`version_code`),
                                KEY `idx_platform_status_code` (`platform`,`status`,`version_code`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COMMENT='App 发布信息表';


CREATE TABLE `login_log` (
                             `id` BIGINT NOT NULL AUTO_INCREMENT,
                             `user_id` BIGINT NOT NULL COMMENT '用户ID',
                             `login_type` varchar(20) NOT NULL COMMENT '登录类型: sms/wechat/apple',
                             `ip` varchar(45) COMMENT '登录IP',
                             `device_id` varchar(128) COMMENT '设备唯一标识',
                             `platform` varchar(10) COMMENT '客户端平台: ios/android',
                             `created_at` datetime DEFAULT CURRENT_TIMESTAMP COMMENT '创建时间',
                             PRIMARY KEY (`id`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COMMENT='用户登录日志表';


CREATE TABLE `companion_sessions` (
                                     `session_id` VARCHAR(64) NOT NULL COMMENT '同行会话ID',
                                     `owner_user_id` BIGINT NOT NULL COMMENT '发起人用户ID',
                                     `status` VARCHAR(16) NOT NULL COMMENT 'active / ended',
                                     `visibility` VARCHAR(16) NOT NULL DEFAULT 'private' COMMENT 'private 私密（需 join_token） / public 公开（凭 session_id 即可加入，并出现在附近列表）',
                                     `join_token` VARCHAR(128) NOT NULL COMMENT '加入凭证',
                                     `title` VARCHAR(64) NOT NULL DEFAULT '' COMMENT '会话标题',
                                     `track_type` VARCHAR(32) NOT NULL DEFAULT '' COMMENT '运动类型',
                                     `locate_addr` VARCHAR(255) NOT NULL DEFAULT '' COMMENT '位置信息',
                                     `max_members` INT NOT NULL DEFAULT '8' COMMENT '最大成员数',
	                                     `danmaku_enabled` TINYINT(1) NOT NULL DEFAULT 1 COMMENT '弹幕开关：1=开启 0=关闭',
	                                     `total_distance` DOUBLE NOT NULL DEFAULT 0 COMMENT '同行总里程，单位米',
	                                     `total_duration` BIGINT NOT NULL DEFAULT 0 COMMENT '同行总耗时，单位秒',
	                                     `track_screenshot_url` VARCHAR(255) NOT NULL DEFAULT '' COMMENT '同行轨迹截图文件在对象存储中的地址',
	                                     `actual_participant_count` BIGINT NOT NULL DEFAULT 0 COMMENT '实际参与同行人数',
	                                     `started_at` DATETIME(6) NOT NULL COMMENT '开始时间',
	                                     `ended_at` DATETIME(6) DEFAULT NULL COMMENT '结束时间',
	                                     `end_reason` VARCHAR(32) NOT NULL DEFAULT '' COMMENT '结束原因：owner_ended / all_members_left / inactive_timeout / max_duration_exceeded',
	                                     `end_source` VARCHAR(32) NOT NULL DEFAULT '' COMMENT '结束来源：owner / member_flow / auto_close',
	                                     `end_operator_user_id` BIGINT NOT NULL DEFAULT 0 COMMENT '结束操作用户ID；自动收尾为0',
	                                     `created_at` DATETIME(6) NOT NULL,
	                                     `updated_at` DATETIME(6) NOT NULL,
                                     PRIMARY KEY (`session_id`),
                                     UNIQUE KEY `uk_companion_join_token` (`join_token`),
                                     KEY `idx_companion_owner_status` (`owner_user_id`, `status`),
                                     KEY `idx_companion_session_status_ended` (`status`, `ended_at`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COMMENT='同行会话表';


CREATE TABLE `companion_session_members` (
                                            `session_id` VARCHAR(64) NOT NULL COMMENT '同行会话ID',
                                            `user_id` BIGINT NOT NULL COMMENT '成员用户ID',
                                            `role` VARCHAR(16) NOT NULL COMMENT 'owner / member',
                                            `member_status` VARCHAR(16) NOT NULL COMMENT 'joined / left / kicked / ended',
                                            `presence_status` VARCHAR(16) NOT NULL COMMENT 'online / offline',
                                            `joined_at` DATETIME(6) NOT NULL COMMENT '加入时间',
                                            `left_at` DATETIME(6) DEFAULT NULL COMMENT '离开时间',
                                            `last_seen_at` DATETIME(6) DEFAULT NULL COMMENT '最近活跃时间',
                                            `mqtt_client_id` VARCHAR(128) NOT NULL DEFAULT '' COMMENT '后续 MQTT client_id 预留',
                                            `mqtt_principal` VARCHAR(128) NOT NULL DEFAULT '' COMMENT '后续 MQTT principal 预留',
                                            PRIMARY KEY (`session_id`, `user_id`),
                                            KEY `idx_companion_member_status` (`session_id`, `member_status`),
                                            KEY `idx_companion_presence` (`session_id`, `presence_status`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COMMENT='同行会话成员表';


CREATE TABLE `companion_live_positions` (
                                           `session_id` VARCHAR(64) NOT NULL COMMENT '同行会话ID',
                                           `user_id` BIGINT NOT NULL COMMENT '成员用户ID',
                                           `track_id` VARCHAR(64) DEFAULT NULL COMMENT '关联轨迹ID（可空）',
                                           `latitude` DOUBLE NOT NULL COMMENT '纬度',
                                           `longitude` DOUBLE NOT NULL COMMENT '经度',
                                           `coordinate_system` VARCHAR(16) NOT NULL COMMENT '坐标系',
                                           `speed_kmh` DOUBLE NOT NULL DEFAULT '0' COMMENT '速度km/h',
                                           `heading` DOUBLE NOT NULL DEFAULT '0' COMMENT '朝向',
                                           `accuracy_m` DOUBLE NOT NULL DEFAULT '0' COMMENT '精度(米)',
                                           `altitude` DOUBLE NOT NULL DEFAULT '0' COMMENT '海拔',
                                           `recorded_at` DATETIME(6) NOT NULL COMMENT '采样时间',
                                           `seq` BIGINT NOT NULL DEFAULT '0' COMMENT '客户端单调序号',
                                           `source` VARCHAR(16) NOT NULL DEFAULT 'http' COMMENT '快照来源：http / mqtt',
                                           `created_at` DATETIME(6) NOT NULL,
                                           `updated_at` DATETIME(6) NOT NULL,
                                           PRIMARY KEY (`session_id`, `user_id`),
                                           KEY `idx_companion_positions_recorded` (`session_id`, `recorded_at`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COMMENT='同行会话最新位置快照表';


CREATE TABLE `companion_danmakus` (
                                     `id` BIGINT UNSIGNED NOT NULL AUTO_INCREMENT,
                                     `session_id` VARCHAR(64) NOT NULL COMMENT '同行会话ID',
                                     `user_id` BIGINT NOT NULL COMMENT '发送者用户ID',
                                     `content` VARCHAR(200) NOT NULL COMMENT '弹幕文本内容',
                                     `created_at` DATETIME(6) NOT NULL COMMENT '入库时间',
                                     PRIMARY KEY (`id`),
                                     KEY `idx_companion_danmaku_session_time` (`session_id`, `created_at`),
                                     KEY `idx_companion_danmaku_session_user_time` (`session_id`, `user_id`, `created_at`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COMMENT='同行文字弹幕表';

CREATE TABLE `companion_events` (
                                    `id` BIGINT UNSIGNED NOT NULL AUTO_INCREMENT,
                                    `session_id` VARCHAR(64) NOT NULL COMMENT '同行会话ID',
                                    `owner_user_id` BIGINT NOT NULL COMMENT '房主用户ID',
                                    `event_type` VARCHAR(32) NOT NULL COMMENT '事件类型',
                                    `target_user_id` BIGINT NOT NULL DEFAULT 0 COMMENT '事件关联成员用户ID；无关联成员为0',
                                    `title` VARCHAR(64) NOT NULL DEFAULT '' COMMENT '事件标题',
                                    `content` VARCHAR(500) NOT NULL DEFAULT '' COMMENT '事件内容',
                                    `event_time` DATETIME(6) NOT NULL COMMENT '事件发生时间',
                                    `client_event_id` VARCHAR(128) NOT NULL COMMENT '客户端幂等事件ID',
                                    `metadata_json` TEXT COMMENT '客户端扩展JSON对象',
                                    `created_at` DATETIME(6) NOT NULL COMMENT '入库时间',
                                    PRIMARY KEY (`id`),
                                    UNIQUE KEY `uk_companion_event_client` (`session_id`, `client_event_id`),
                                    KEY `idx_companion_events_session_time` (`session_id`, `event_time`, `id`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COMMENT='同行关键事件表';

CREATE TABLE `user_feedbacks` (
                                  `id` BIGINT UNSIGNED NOT NULL AUTO_INCREMENT,
                                  `feedback_id` VARCHAR(32) NOT NULL COMMENT '反馈业务ID',
                                  `user_id` BIGINT NOT NULL COMMENT '提交用户ID',
                                  `content` TEXT NOT NULL COMMENT '反馈文字内容',
                                  `images_json` JSON DEFAULT NULL COMMENT '反馈图片元信息，最多3张',
                                  `contact` VARCHAR(128) NOT NULL DEFAULT '' COMMENT '用户补充联系方式',
                                  `app_version` VARCHAR(32) NOT NULL DEFAULT '' COMMENT '客户端版本',
                                  `platform` VARCHAR(32) NOT NULL DEFAULT '' COMMENT '客户端平台',
                                  `device_model` VARCHAR(128) NOT NULL DEFAULT '' COMMENT '设备型号',
                                  `system_version` VARCHAR(64) NOT NULL DEFAULT '' COMMENT '系统版本',
                                  `status` VARCHAR(32) NOT NULL DEFAULT 'pending' COMMENT 'pending / processing / resolved / ignored',
                                  `reply` TEXT COMMENT '运营处理备注',
                                  `created_at` DATETIME(6) NOT NULL,
                                  `updated_at` DATETIME(6) NOT NULL,
                                  PRIMARY KEY (`id`),
                                  UNIQUE KEY `uk_feedback_id` (`feedback_id`),
                                  KEY `idx_feedback_user_created` (`user_id`, `created_at`, `feedback_id`),
                                  KEY `idx_feedback_status_created` (`status`, `created_at`, `feedback_id`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COMMENT='用户意见反馈表';
