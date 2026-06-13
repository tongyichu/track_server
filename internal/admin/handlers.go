package admin

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/cloudwego/hertz/pkg/app"
	"github.com/cloudwego/hertz/pkg/common/utils"

	"github.com/tongyichu/track_server/internal/models"
	"github.com/tongyichu/track_server/internal/repository"
	"github.com/tongyichu/track_server/internal/service"
)

// Handler 聚合管理后台需要的业务依赖。
type Handler struct {
	releaseSvc    *service.AppReleaseService
	stsSvc        *service.OSSTokenService
	auth          *Authenticator
	userRepo      repository.UserRepository
	userSvc       *service.UserService
	trackRepo     repository.TrackRepository
	companionRepo repository.CompanionRepository
	analyticsRepo repository.AnalyticsRepository
	feedbackSvc   *service.FeedbackService
	// staticRoot 是服务端本地静态资源根目录（通常为 <LogDir>/static）。
	// 管理后台上传的安装包会落到 <staticRoot>/release/<platform>/ 下，
	// 并通过 /api/v1/static/release/<platform>/<file> 对外下发。
	staticRoot string
}

// NewHandler 构造管理后台 Handler。
func NewHandler(
	releaseSvc *service.AppReleaseService,
	stsSvc *service.OSSTokenService,
	auth *Authenticator,
	staticRoot string,
	userRepo repository.UserRepository,
	trackRepo repository.TrackRepository,
	companionRepo repository.CompanionRepository,
	analyticsRepo repository.AnalyticsRepository,
	userSvc *service.UserService,
	feedbackSvc *service.FeedbackService,
) *Handler {
	return &Handler{
		releaseSvc:    releaseSvc,
		stsSvc:        stsSvc,
		auth:          auth,
		staticRoot:    staticRoot,
		userRepo:      userRepo,
		userSvc:       userSvc,
		trackRepo:     trackRepo,
		companionRepo: companionRepo,
		analyticsRepo: analyticsRepo,
		feedbackSvc:   feedbackSvc,
	}
}

// ----- 发布列表 -----

// ListReleases 处理 GET /admin/api/releases
func (h *Handler) ListReleases(ctx context.Context, c *app.RequestContext) {
	if h == nil || h.releaseSvc == nil {
		c.JSON(http.StatusInternalServerError, utils.H{"error": "release service not configured"})
		return
	}
	filter := models.AppReleaseListFilter{
		Platform: models.AppReleasePlatform(string(c.Query("platform"))),
		Status:   models.AppReleaseStatus(string(c.Query("status"))),
	}
	list, err := h.releaseSvc.List(ctx, filter)
	if err != nil {
		c.JSON(http.StatusInternalServerError, utils.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, utils.H{"code": 0, "data": utils.H{"items": list}})
}

// ----- 发布版本 -----

type publishReleaseBody struct {
	Platform                string `json:"platform"`
	VersionName             string `json:"version_name"`
	VersionCode             int64  `json:"version_code"`
	MinSupportedVersionCode int64  `json:"min_supported_version_code"`
	PackageURL              string `json:"package_url"`
	PackageSize             int64  `json:"package_size"`
	PackageMD5              string `json:"package_md5"`
	ReleaseNotes            string `json:"release_notes"`
	ForceUpdate             bool   `json:"force_update"`
	Status                  string `json:"status"`
}

// PublishRelease 处理 POST /admin/api/releases
func (h *Handler) PublishRelease(ctx context.Context, c *app.RequestContext) {
	if h == nil || h.releaseSvc == nil {
		c.JSON(http.StatusInternalServerError, utils.H{"error": "release service not configured"})
		return
	}
	var body publishReleaseBody
	data, err := c.Body()
	if err != nil || json.Unmarshal(data, &body) != nil {
		c.JSON(http.StatusBadRequest, utils.H{"error": "invalid payload"})
		return
	}

	sess := h.auth.SessionFromRequest(c)
	operatorName := ""
	if sess != nil {
		operatorName = sess.Username
	}

	in := service.PublishReleaseInput{
		Platform:                models.AppReleasePlatform(body.Platform),
		VersionName:             body.VersionName,
		VersionCode:             body.VersionCode,
		MinSupportedVersionCode: body.MinSupportedVersionCode,
		PackageURL:              body.PackageURL,
		PackageSize:             body.PackageSize,
		PackageMD5:              body.PackageMD5,
		ReleaseNotes:            body.ReleaseNotes,
		ForceUpdate:             body.ForceUpdate,
		Status:                  models.AppReleaseStatus(body.Status),
		OperatorName:            operatorName,
	}
	rel, err := h.releaseSvc.Publish(ctx, in)
	if err != nil {
		var iae *service.InvalidArgumentError
		if errors.As(err, &iae) {
			c.JSON(http.StatusBadRequest, utils.H{"error": iae.Error()})
			return
		}
		c.JSON(http.StatusInternalServerError, utils.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, utils.H{"code": 0, "data": rel})
}

// DeleteRelease 处理 DELETE /admin/api/releases/:id
func (h *Handler) DeleteRelease(ctx context.Context, c *app.RequestContext) {
	if h == nil || h.releaseSvc == nil {
		c.JSON(http.StatusInternalServerError, utils.H{"error": "release service not configured"})
		return
	}
	idStr := c.Param("id")
	id, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil || id <= 0 {
		c.JSON(http.StatusBadRequest, utils.H{"error": "invalid id"})
		return
	}
	if err := h.releaseSvc.Delete(ctx, id); err != nil {
		var iae *service.InvalidArgumentError
		if errors.As(err, &iae) {
			c.JSON(http.StatusBadRequest, utils.H{"error": iae.Error()})
			return
		}
		if errors.Is(err, repository.ErrNotFound) {
			c.JSON(http.StatusNotFound, utils.H{"error": "not found"})
			return
		}
		c.JSON(http.StatusInternalServerError, utils.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, utils.H{"code": 0, "data": utils.H{"status": "ok"}})
}

// ----- 上传凭证 -----

// maxUploadPackageSize 管理后台上传安装包的大小上限：500 MiB。
// APK/IPA 不常超过此值；上限用于防止误操作把大文件打满磁盘。
const maxUploadPackageSize int64 = 500 * 1024 * 1024

// UploadPackage 处理 POST /admin/api/releases/upload-package
//
// 接收 multipart/form-data：
//   - file:     安装包文件（必填）
//   - platform: android / ios（必填）
//
// 成功后把文件写入 <staticRoot>/release/<platform>/<ts>-<safe_name>，
// 返回 { url, size, filename }，其中 url 形如
// "/api/v1/static/release/<platform>/<filename>"，前端填入 package_url 字段即可。
func (h *Handler) UploadPackage(ctx context.Context, c *app.RequestContext) {
	if h == nil {
		c.JSON(http.StatusInternalServerError, utils.H{"error": "admin not configured"})
		return
	}
	if strings.TrimSpace(h.staticRoot) == "" {
		c.JSON(http.StatusInternalServerError, utils.H{"error": "static root not configured"})
		return
	}

	platform := strings.ToLower(strings.TrimSpace(string(c.FormValue("platform"))))
	if platform != string(models.AppReleasePlatformAndroid) && platform != string(models.AppReleasePlatformIOS) {
		c.JSON(http.StatusBadRequest, utils.H{"error": "platform must be android or ios"})
		return
	}

	fileHeader, err := c.FormFile("file")
	if err != nil || fileHeader == nil {
		c.JSON(http.StatusBadRequest, utils.H{"error": "file is required"})
		return
	}
	if fileHeader.Size <= 0 {
		c.JSON(http.StatusBadRequest, utils.H{"error": "empty file"})
		return
	}
	if fileHeader.Size > maxUploadPackageSize {
		c.JSON(http.StatusRequestEntityTooLarge, utils.H{
			"error": fmt.Sprintf("file too large: %d bytes, max %d", fileHeader.Size, maxUploadPackageSize),
		})
		return
	}

	safeName := sanitizeFileName(fileHeader.Filename)
	if safeName == "" {
		safeName = "app"
	}
	finalName := fmt.Sprintf("%d-%s", time.Now().UnixMilli(), safeName)

	dstDir := filepath.Join(h.staticRoot, "release", platform)
	if err := os.MkdirAll(dstDir, 0o755); err != nil {
		c.JSON(http.StatusInternalServerError, utils.H{"error": "mkdir failed: " + err.Error()})
		return
	}
	dstPath := filepath.Join(dstDir, finalName)

	src, err := fileHeader.Open()
	if err != nil {
		c.JSON(http.StatusInternalServerError, utils.H{"error": "open upload failed: " + err.Error()})
		return
	}
	defer func() { _ = src.Close() }()

	dst, err := os.OpenFile(dstPath, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o644)
	if err != nil {
		c.JSON(http.StatusInternalServerError, utils.H{"error": "create file failed: " + err.Error()})
		return
	}
	written, copyErr := io.Copy(dst, src)
	closeErr := dst.Close()
	if copyErr != nil {
		_ = os.Remove(dstPath)
		c.JSON(http.StatusInternalServerError, utils.H{"error": "write file failed: " + copyErr.Error()})
		return
	}
	if closeErr != nil {
		_ = os.Remove(dstPath)
		c.JSON(http.StatusInternalServerError, utils.H{"error": "close file failed: " + closeErr.Error()})
		return
	}

	publicURL := "/api/v1/static/release/" + platform + "/" + finalName
	c.JSON(http.StatusOK, utils.H{"code": 0, "data": utils.H{
		"url":      publicURL,
		"size":     written,
		"filename": finalName,
	}})
}

// sanitizeFileName 把上传文件名里可能导致路径穿越/乱码的字符替换成下划线，
// 仅保留 [A-Za-z0-9._-]，其他一律替换。
func sanitizeFileName(name string) string {
	name = filepath.Base(name)
	// 去掉前导 '.'，防止被当作隐藏文件
	name = strings.TrimLeft(name, ".")
	var b strings.Builder
	b.Grow(len(name))
	for _, r := range name {
		switch {
		case r >= 'a' && r <= 'z',
			r >= 'A' && r <= 'Z',
			r >= '0' && r <= '9',
			r == '.' || r == '_' || r == '-':
			b.WriteRune(r)
		default:
			b.WriteByte('_')
		}
	}
	return b.String()
}

// ----- 用户管理 -----

// adminListLimit 是管理后台列表接口的默认/最大每页条数。
const (
	adminListDefaultLimit = 20
	adminListMaxLimit     = 100
)

// parseAdminListLimit 解析管理后台 list 接口的 limit query。
func parseAdminListLimit(raw string) int {
	if strings.TrimSpace(raw) == "" {
		return adminListDefaultLimit
	}
	v, err := strconv.Atoi(raw)
	if err != nil || v <= 0 {
		return adminListDefaultLimit
	}
	if v > adminListMaxLimit {
		return adminListMaxLimit
	}
	return v
}

// ListUsers 处理 GET /admin/api/users
//
// query：
//   - cursor_created_at: RFC3339 时间，上一页最后一条的 created_at；
//   - cursor_id:         上一页最后一条的用户 ID；
//   - limit:             每页条数（默认 20，最大 100）。
//
// 返回：{ items: []User, total, next_cursor: {created_at, id}, has_more }
func (h *Handler) ListUsers(ctx context.Context, c *app.RequestContext) {
	if h == nil || h.userRepo == nil {
		c.JSON(http.StatusInternalServerError, utils.H{"error": "user repository not configured"})
		return
	}
	limit := parseAdminListLimit(string(c.Query("limit")))
	var cursor *models.UserListCursor
	if rawAt := strings.TrimSpace(string(c.Query("cursor_created_at"))); rawAt != "" {
		t, err := time.Parse(time.RFC3339Nano, rawAt)
		if err != nil {
			c.JSON(http.StatusBadRequest, utils.H{"error": "invalid cursor_created_at"})
			return
		}
		idStr := strings.TrimSpace(string(c.Query("cursor_id")))
		id, err := strconv.ParseInt(idStr, 10, 64)
		if err != nil || id <= 0 {
			c.JSON(http.StatusBadRequest, utils.H{"error": "invalid cursor_id"})
			return
		}
		cursor = &models.UserListCursor{CreatedAt: t, ID: id}
	}
	items, err := h.userRepo.ListAll(ctx, cursor, limit)
	if err != nil {
		c.JSON(http.StatusInternalServerError, utils.H{"error": err.Error()})
		return
	}
	// 与业务接口口径保持一致：把 OSS 上的 avatar_url 改写为服务器本地缓存地址
	// （/api/v1/static/avatars/...），未配置头像的用户回退到默认头像。
	if h.userSvc != nil {
		for _, u := range items {
			h.userSvc.DecorateAvatar(ctx, u)
			u.AvatarURL = rewriteAdminStaticURL(u.AvatarURL)
		}
	}
	total, err := h.userRepo.CountAll(ctx)
	if err != nil {
		c.JSON(http.StatusInternalServerError, utils.H{"error": err.Error()})
		return
	}
	resp := utils.H{
		"items":    items,
		"total":    total,
		"has_more": len(items) == limit,
	}
	if len(items) == limit {
		last := items[len(items)-1]
		resp["next_cursor"] = utils.H{
			"created_at": last.CreatedAt.Format(time.RFC3339Nano),
			"id":         last.ID,
		}
	}
	c.JSON(http.StatusOK, utils.H{"code": 0, "data": resp})
}

// ----- 轨迹管理 -----

// ListTracks 处理 GET /admin/api/tracks
//
// query：
//   - cursor_start_time: RFC3339 时间，上一页最后一条的 start_time；
//   - cursor_id:         上一页最后一条轨迹 ID；
//   - limit:             每页条数（默认 20，最大 100）。
func (h *Handler) ListTracks(ctx context.Context, c *app.RequestContext) {
	if h == nil || h.trackRepo == nil {
		c.JSON(http.StatusInternalServerError, utils.H{"error": "track repository not configured"})
		return
	}
	limit := parseAdminListLimit(string(c.Query("limit")))
	var cursor *models.TrackListCursor
	if rawAt := strings.TrimSpace(string(c.Query("cursor_start_time"))); rawAt != "" {
		t, err := time.Parse(time.RFC3339Nano, rawAt)
		if err != nil {
			c.JSON(http.StatusBadRequest, utils.H{"error": "invalid cursor_start_time"})
			return
		}
		id := strings.TrimSpace(string(c.Query("cursor_id")))
		if id == "" {
			c.JSON(http.StatusBadRequest, utils.H{"error": "invalid cursor_id"})
			return
		}
		cursor = &models.TrackListCursor{StartTime: t, ID: id}
	}
	items, err := h.trackRepo.ListAll(ctx, cursor, limit)
	if err != nil {
		c.JSON(http.StatusInternalServerError, utils.H{"error": err.Error()})
		return
	}
	total, err := h.trackRepo.CountAll(ctx)
	if err != nil {
		c.JSON(http.StatusInternalServerError, utils.H{"error": err.Error()})
		return
	}
	resp := utils.H{
		"items":    items,
		"total":    total,
		"has_more": len(items) == limit,
	}
	if len(items) == limit {
		last := items[len(items)-1]
		resp["next_cursor"] = utils.H{
			"start_time": last.StartTime.Format(time.RFC3339Nano),
			"id":         last.ID,
		}
	}
	c.JSON(http.StatusOK, utils.H{"code": 0, "data": resp})
}

// ----- 同行管理 -----

// ListCompanions 处理 GET /admin/api/companions
//
// query：
//   - cursor_started_at: RFC3339 时间，上一页最后一条的 started_at；
//   - cursor_session_id: 上一页最后一条会话的 session_id；
//   - limit:             每页条数（默认 20，最大 100）。
func (h *Handler) ListCompanions(ctx context.Context, c *app.RequestContext) {
	if h == nil || h.companionRepo == nil {
		c.JSON(http.StatusInternalServerError, utils.H{"error": "companion repository not configured"})
		return
	}
	limit := parseAdminListLimit(string(c.Query("limit")))
	var cursor *models.CompanionSessionListCursor
	if rawAt := strings.TrimSpace(string(c.Query("cursor_started_at"))); rawAt != "" {
		t, err := time.Parse(time.RFC3339Nano, rawAt)
		if err != nil {
			c.JSON(http.StatusBadRequest, utils.H{"error": "invalid cursor_started_at"})
			return
		}
		sid := strings.TrimSpace(string(c.Query("cursor_session_id")))
		if sid == "" {
			c.JSON(http.StatusBadRequest, utils.H{"error": "invalid cursor_session_id"})
			return
		}
		cursor = &models.CompanionSessionListCursor{StartedAt: t, SessionID: sid}
	}
	items, err := h.companionRepo.ListAllSessions(ctx, cursor, limit)
	if err != nil {
		c.JSON(http.StatusInternalServerError, utils.H{"error": err.Error()})
		return
	}
	total, err := h.companionRepo.CountAllSessions(ctx)
	if err != nil {
		c.JSON(http.StatusInternalServerError, utils.H{"error": err.Error()})
		return
	}
	resp := utils.H{
		"items":    items,
		"total":    total,
		"has_more": len(items) == limit,
	}
	if len(items) == limit {
		last := items[len(items)-1]
		resp["next_cursor"] = utils.H{
			"started_at": last.StartedAt.Format(time.RFC3339Nano),
			"session_id": last.SessionID,
		}
	}
	c.JSON(http.StatusOK, utils.H{"code": 0, "data": resp})
}

// companionMemberDetail 是同行详情中的成员展示信息（包含头像、昵称等）。
type companionMemberDetail struct {
	UserID         int64                          `json:"user_id"`
	Nickname       string                         `json:"nickname"`
	AvatarURL      string                         `json:"avatar_url"`
	Role           models.CompanionMemberRole     `json:"role"`
	MemberStatus   models.CompanionMemberStatus   `json:"member_status"`
	PresenceStatus models.CompanionPresenceStatus `json:"presence_status"`
	JoinedAt       time.Time                      `json:"joined_at"`
	LeftAt         time.Time                      `json:"left_at,omitempty"`
	LastSeenAt     time.Time                      `json:"last_seen_at,omitempty"`
}

// GetCompanionDetail 处理 GET /admin/api/companions/:session_id
//
// 返回：
//
//	{
//	  session:        CompanionSession,
//	  members:        []companionMemberDetail,    // 含昵称/头像（已改写为本地缓存地址）
//	  live_positions: []*CompanionLivePosition,
//	  danmakus:       []*CompanionDanmaku,         // 最近 200 条
//	}
func (h *Handler) GetCompanionDetail(ctx context.Context, c *app.RequestContext) {
	if h == nil || h.companionRepo == nil {
		c.JSON(http.StatusInternalServerError, utils.H{"error": "companion repository not configured"})
		return
	}
	sessionID := strings.TrimSpace(c.Param("session_id"))
	if sessionID == "" {
		c.JSON(http.StatusBadRequest, utils.H{"error": "session_id is required"})
		return
	}
	session, err := h.companionRepo.FindSessionByID(ctx, sessionID)
	if err != nil {
		if errors.Is(err, repository.ErrNotFound) {
			c.JSON(http.StatusNotFound, utils.H{"error": "session not found"})
			return
		}
		c.JSON(http.StatusInternalServerError, utils.H{"error": err.Error()})
		return
	}
	rawMembers, err := h.companionRepo.ListMembers(ctx, sessionID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, utils.H{"error": err.Error()})
		return
	}
	members := make([]companionMemberDetail, 0, len(rawMembers))
	for _, m := range rawMembers {
		if m == nil {
			continue
		}
		detail := companionMemberDetail{
			UserID:         m.UserID,
			Role:           m.Role,
			MemberStatus:   m.MemberStatus,
			PresenceStatus: m.PresenceStatus,
			JoinedAt:       m.JoinedAt,
			LeftAt:         m.LeftAt,
			LastSeenAt:     m.LastSeenAt,
		}
		if h.userRepo != nil {
			if u, uerr := h.userRepo.FindByID(ctx, m.UserID); uerr == nil && u != nil {
				if h.userSvc != nil {
					h.userSvc.DecorateAvatar(ctx, u)
				}
				detail.Nickname = u.Nickname
				detail.AvatarURL = rewriteAdminStaticURL(u.AvatarURL)
			}
		}
		members = append(members, detail)
	}
	positions, err := h.companionRepo.ListPositions(ctx, sessionID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, utils.H{"error": err.Error()})
		return
	}
	danmakus, err := h.companionRepo.ListDanmakusBySessionID(ctx, sessionID, 200)
	if err != nil {
		c.JSON(http.StatusInternalServerError, utils.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, utils.H{"code": 0, "data": utils.H{
		"session":        session,
		"members":        members,
		"live_positions": positions,
		"danmakus":       danmakus,
	}})
}

// GetStaticAsset 处理 GET /admin/api/static/*filepath
//
// 业务静态资源 /api/v1/static/* 需要业务 JWT；管理后台只持有 admin_session，
// 因此后台页面通过这个已鉴权代理读取同一个 staticRoot 下的头像等资源。
func (h *Handler) GetStaticAsset(ctx context.Context, c *app.RequestContext) {
	if h == nil || strings.TrimSpace(h.staticRoot) == "" {
		c.JSON(http.StatusInternalServerError, utils.H{"error": "static root not configured"})
		return
	}
	rel, ok := cleanAdminStaticPath(c.Param("filepath"))
	if !ok {
		c.JSON(http.StatusBadRequest, utils.H{"error": "invalid path"})
		return
	}
	fullPath := filepath.Join(h.staticRoot, filepath.FromSlash(rel))
	root, err := filepath.Abs(h.staticRoot)
	if err != nil {
		c.JSON(http.StatusInternalServerError, utils.H{"error": err.Error()})
		return
	}
	absPath, err := filepath.Abs(fullPath)
	if err != nil {
		c.JSON(http.StatusInternalServerError, utils.H{"error": err.Error()})
		return
	}
	if r, err := filepath.Rel(root, absPath); err != nil || r == "." || strings.HasPrefix(r, ".."+string(filepath.Separator)) || r == ".." {
		c.JSON(http.StatusBadRequest, utils.H{"error": "invalid path"})
		return
	}
	data, err := os.ReadFile(absPath)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) && isAdminDefaultAvatarPath(rel) {
			c.Data(http.StatusOK, "image/svg+xml", []byte(defaultAdminAvatarSVG(path.Base(rel))))
			return
		}
		if errors.Is(err, os.ErrNotExist) {
			c.JSON(http.StatusNotFound, utils.H{"error": "not found"})
			return
		}
		c.JSON(http.StatusInternalServerError, utils.H{"error": err.Error()})
		return
	}
	c.Data(http.StatusOK, contentTypeFor(rel), data)
}

func rewriteAdminStaticURL(raw string) string {
	const apiStaticPrefix = "/api/v1/static/"
	if strings.HasPrefix(raw, apiStaticPrefix) {
		return "/admin/api/static/" + strings.TrimPrefix(raw, apiStaticPrefix)
	}
	return raw
}

func cleanAdminStaticPath(raw string) (string, bool) {
	raw = strings.TrimSpace(strings.ReplaceAll(raw, "\\", "/"))
	raw = strings.TrimPrefix(raw, "/")
	if raw == "" {
		return "", false
	}
	for _, part := range strings.Split(raw, "/") {
		if part == ".." {
			return "", false
		}
	}
	cleaned := strings.TrimPrefix(path.Clean("/"+raw), "/")
	if cleaned == "." || cleaned == "" || strings.HasPrefix(cleaned, "../") || strings.Contains(cleaned, "/../") {
		return "", false
	}
	return cleaned, true
}

func isAdminDefaultAvatarPath(rel string) bool {
	switch rel {
	case "default_avatars/girl_01.png",
		"default_avatars/girl_2.png",
		"default_avatars/boy_01.png",
		"default_avatars/boy_02.png":
		return true
	default:
		return false
	}
}

func defaultAdminAvatarSVG(name string) string {
	fill := "#64748b"
	switch name {
	case "girl_01.png":
		fill = "#db2777"
	case "girl_2.png":
		fill = "#7c3aed"
	case "boy_01.png":
		fill = "#2563eb"
	case "boy_02.png":
		fill = "#059669"
	}
	return fmt.Sprintf(`<svg xmlns="http://www.w3.org/2000/svg" width="96" height="96" viewBox="0 0 96 96"><rect width="96" height="96" rx="48" fill="%s"/><circle cx="48" cy="36" r="18" fill="#fff" fill-opacity=".9"/><path d="M19 84c5-18 16-28 29-28s24 10 29 28" fill="#fff" fill-opacity=".9"/></svg>`, fill)
}

// ----- 意见反馈管理 -----

// ListFeedbacks 处理 GET /admin/api/feedbacks
//
// query：
//   - status: pending / processing / resolved / ignored，可空；
//   - cursor: 上一页返回的 next_cursor；
//   - limit:  每页条数（默认 20，最大 100）。
func (h *Handler) ListFeedbacks(ctx context.Context, c *app.RequestContext) {
	if h == nil || h.feedbackSvc == nil {
		c.JSON(http.StatusInternalServerError, utils.H{"error": "feedback service not configured"})
		return
	}
	cursor, err := service.ParseFeedbackCursor(string(c.Query("cursor")))
	if err != nil {
		c.JSON(http.StatusBadRequest, utils.H{"error": err.Error()})
		return
	}
	page, err := h.feedbackSvc.ListOps(ctx, models.FeedbackStatus(strings.TrimSpace(string(c.Query("status")))), cursor, parseAdminListLimit(string(c.Query("limit"))))
	if err != nil {
		writeAdminFeedbackError(c, err)
		return
	}
	h.rewriteFeedbackImageURLs(page.Items)
	c.JSON(http.StatusOK, utils.H{"code": 0, "data": page})
}

// GetFeedback 处理 GET /admin/api/feedbacks/:feedback_id
func (h *Handler) GetFeedback(ctx context.Context, c *app.RequestContext) {
	if h == nil || h.feedbackSvc == nil {
		c.JSON(http.StatusInternalServerError, utils.H{"error": "feedback service not configured"})
		return
	}
	item, err := h.feedbackSvc.GetOps(ctx, c.Param("feedback_id"))
	if err != nil {
		writeAdminFeedbackError(c, err)
		return
	}
	h.rewriteFeedbackImageURLs([]*models.Feedback{item})
	c.JSON(http.StatusOK, utils.H{"code": 0, "data": item})
}

type updateFeedbackStatusBody struct {
	Status models.FeedbackStatus `json:"status"`
	Reply  string                `json:"reply"`
}

// UpdateFeedbackStatus 处理 PUT /admin/api/feedbacks/:feedback_id/status
func (h *Handler) UpdateFeedbackStatus(ctx context.Context, c *app.RequestContext) {
	if h == nil || h.feedbackSvc == nil {
		c.JSON(http.StatusInternalServerError, utils.H{"error": "feedback service not configured"})
		return
	}
	var body updateFeedbackStatusBody
	data, err := c.Body()
	if err != nil || json.Unmarshal(data, &body) != nil {
		c.JSON(http.StatusBadRequest, utils.H{"error": "invalid payload"})
		return
	}
	if err := h.feedbackSvc.UpdateStatus(ctx, service.UpdateFeedbackStatusInput{
		FeedbackID: c.Param("feedback_id"),
		Status:     body.Status,
		Reply:      body.Reply,
	}); err != nil {
		writeAdminFeedbackError(c, err)
		return
	}
	c.JSON(http.StatusOK, utils.H{"code": 0, "data": utils.H{"status": "ok"}})
}

// GetFeedbackImage 处理 GET /admin/api/feedbacks/:feedback_id/images/:image_id
func (h *Handler) GetFeedbackImage(ctx context.Context, c *app.RequestContext) {
	if h == nil || h.feedbackSvc == nil {
		c.JSON(http.StatusInternalServerError, utils.H{"error": "feedback service not configured"})
		return
	}
	file, err := h.feedbackSvc.GetImageFile(ctx, 0, c.Param("feedback_id"), c.Param("image_id"), true)
	if err != nil {
		writeAdminFeedbackError(c, err)
		return
	}
	data, err := os.ReadFile(file.Path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			c.JSON(http.StatusNotFound, utils.H{"error": "not found"})
			return
		}
		c.JSON(http.StatusInternalServerError, utils.H{"error": err.Error()})
		return
	}
	c.Response.SetStatusCode(http.StatusOK)
	c.Response.Header.SetContentType(file.MimeType)
	c.Response.SetBody(data)
}

func (h *Handler) rewriteFeedbackImageURLs(items []*models.Feedback) {
	for _, item := range items {
		if item == nil {
			continue
		}
		for i := range item.Images {
			item.Images[i].URL = "/admin/api/feedbacks/" + item.FeedbackID + "/images/" + item.Images[i].ImageID
		}
	}
}

func writeAdminFeedbackError(c *app.RequestContext, err error) {
	var iae *service.InvalidArgumentError
	switch {
	case errors.As(err, &iae),
		errors.Is(err, service.ErrFeedbackReplyRequired):
		c.JSON(http.StatusBadRequest, utils.H{"error": err.Error()})
	case errors.Is(err, repository.ErrNotFound):
		c.JSON(http.StatusNotFound, utils.H{"error": "not found"})
	case errors.Is(err, repository.ErrForbidden):
		c.JSON(http.StatusForbidden, utils.H{"error": "forbidden"})
	default:
		c.JSON(http.StatusInternalServerError, utils.H{"error": err.Error()})
	}
}

// ----- 埋点同步摘要 -----

// ListAnalyticsSyncSummaries 处理 GET /admin/api/analytics/sync-summaries
//
// query：
//   - status: success / partial / failed，可空；
//   - limit:  每页条数（默认 20，最大 100）；
//   - offset: 起始偏移量（默认 0）。
func (h *Handler) ListAnalyticsSyncSummaries(ctx context.Context, c *app.RequestContext) {
	if h == nil || h.analyticsRepo == nil {
		c.JSON(http.StatusInternalServerError, utils.H{"error": "analytics repository not configured"})
		return
	}
	limit := parseAdminListLimit(string(c.Query("limit")))
	offset := parseNonNegativeInt(string(c.Query("offset")))
	status := strings.TrimSpace(string(c.Query("status")))
	items, err := h.analyticsRepo.ListSyncSummaries(ctx, status, limit, offset)
	if err != nil {
		c.JSON(http.StatusInternalServerError, utils.H{"error": err.Error()})
		return
	}
	total, err := h.analyticsRepo.CountSyncSummaries(ctx, status)
	if err != nil {
		c.JSON(http.StatusInternalServerError, utils.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, utils.H{"code": 0, "data": utils.H{
		"items":       items,
		"total":       total,
		"limit":       limit,
		"offset":      offset,
		"has_more":    int64(offset+len(items)) < total,
		"next_offset": offset + len(items),
	}})
}

func parseNonNegativeInt(raw string) int {
	if strings.TrimSpace(raw) == "" {
		return 0
	}
	v, err := strconv.Atoi(raw)
	if err != nil || v < 0 {
		return 0
	}
	return v
}

// GetReleaseUploadCredential 处理 GET /admin/api/releases/upload-token
//
// 用于浏览器直传 APK 到 OSS：返回的是仅允许写入 release/ 目录的临时凭证。
// 可选 query：sub_dir，会在 release/ 下再加一层子目录，例如 sub_dir=android => release/android/。
func (h *Handler) GetReleaseUploadCredential(ctx context.Context, c *app.RequestContext) {
	if h == nil {
		c.JSON(http.StatusInternalServerError, utils.H{"error": "admin not configured"})
		return
	}
	if h.stsSvc == nil {
		c.JSON(http.StatusInternalServerError, utils.H{"error": "oss sts not configured"})
		return
	}
	subDir := string(c.Query("sub_dir"))
	cred, err := h.stsSvc.GetReleaseUploadCredential(subDir)
	if err != nil {
		c.JSON(http.StatusInternalServerError, utils.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, utils.H{"code": 0, "data": cred})
}
