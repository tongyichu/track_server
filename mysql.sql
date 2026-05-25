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
                                 `city_code` varchar(16) NOT NULL DEFAULT '' COMMENT '城市Code',
                                 `locate_addr` varchar(128) NOT NULL DEFAULT '' COMMENT '轨迹的具体位置信息',
                                `track_type` varchar(32) NOT NULL DEFAULT '' COMMENT '轨迹类型，如徒步/跑步/骑车/自驾',
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


CREATE TABLE `track_navigations` (
                                     `id` BIGINT UNSIGNED NOT NULL AUTO_INCREMENT,
                                     `track_id` VARCHAR(64) NOT NULL COMMENT '轨迹ID',
                                     `navigator_user_id` BIGINT NOT NULL COMMENT '导航使用者用户ID',
                                     `created_at` DATETIME(6) NOT NULL DEFAULT CURRENT_TIMESTAMP(6) COMMENT '使用时间',
                                     PRIMARY KEY (`id`),
                                     KEY `idx_nav_track` (`track_id`),
                                     KEY `idx_nav_user` (`navigator_user_id`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COMMENT='轨迹导航使用记录表';


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
                                     `join_token` VARCHAR(128) NOT NULL COMMENT '加入凭证',
                                     `join_token_expire_at` DATETIME(6) NOT NULL COMMENT '加入凭证过期时间',
                                     `title` VARCHAR(64) NOT NULL DEFAULT '' COMMENT '会话标题',
                                     `max_members` INT NOT NULL DEFAULT '8' COMMENT '最大成员数',
                                     `started_at` DATETIME(6) NOT NULL COMMENT '开始时间',
                                     `ended_at` DATETIME(6) DEFAULT NULL COMMENT '结束时间',
                                     `created_at` DATETIME(6) NOT NULL,
                                     `updated_at` DATETIME(6) NOT NULL,
                                     PRIMARY KEY (`session_id`),
                                     UNIQUE KEY `uk_companion_join_token` (`join_token`),
                                     KEY `idx_companion_owner_status` (`owner_user_id`, `status`)
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
