CREATE TABLE `users` (
                          `id` BIGINT NOT NULL,
                          `nickname` varchar(255) NOT NULL DEFAULT '' COMMENT '用户昵称',
                          `avatar_url` varchar(255) COMMENT '用户头像URI',
                          `signature` text,
                          `phone` varchar(32) NOT NULL DEFAULT '' COMMENT '用户手机号',
                          `client_language` varchar(64) NOT NULL DEFAULT '' COMMENT '客户端语言',
                          `created_at` datetime(6) NOT NULL,
                          `updated_at` datetime(6) NOT NULL,
                          PRIMARY KEY (`id`),
                          KEY `idx_users_updated_at` (`updated_at`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COMMENT='用户基础资料表';


CREATE TABLE `track_records` (
                                 `id` VARCHAR(64) NOT NULL,
                                 `user_id` bigint unsigned NOT NULL COMMENT '用户ID',
                                 `title` varchar(128) NOT NULL DEFAULT '' COMMENT '轨迹名称',
                                 `start_time` datetime NOT NULL COMMENT '开始时间',
                                 `end_time` datetime COMMENT '结束时间',
                                 `distance` decimal(10,2) DEFAULT '0.00' COMMENT '总距离(米)',
                                 `duration` int unsigned DEFAULT '0' COMMENT '运动耗时(秒)',
                                 `elevation_gain` int DEFAULT '0' COMMENT '累计爬升(米)',
                                 `raw_track_url` varchar(255) COMMENT '指向对象存储中原始轨迹点文件(JSON/GeoJSON)的URL',
                                 `track_screenshot_url` varchar(255) COMMENT '轨迹截图文件在对象存储中的地址',
                                 `is_running` tinyint NOT NULL DEFAULT '1' COMMENT '表示轨迹是否仍处于进行中: 0-未运行, 1-运行中',
                                 `status` tinyint NOT NULL DEFAULT '1' COMMENT '状态: 0-删除, 1-正常, 2-私密',
                                 `avg_speed_kmh` double NOT NULL DEFAULT '1' COMMENT '平均速度，单位 km/h',
                                 `created_at` timestamp NOT NULL DEFAULT CURRENT_TIMESTAMP,
                                 `updated_at` timestamp NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
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
