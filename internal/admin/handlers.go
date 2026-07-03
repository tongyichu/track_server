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
	"unicode/utf8"

	"github.com/cloudwego/hertz/pkg/app"
	"github.com/cloudwego/hertz/pkg/common/utils"

	"github.com/tongyichu/track_server/internal/models"
	"github.com/tongyichu/track_server/internal/repository"
	"github.com/tongyichu/track_server/internal/service"
)

// Handler 聚合管理后台需要的业务依赖。
type Handler struct {
	releaseSvc         *service.AppReleaseService
	stsSvc             *service.OSSTokenService
	auth               *Authenticator
	userRepo           repository.UserRepository
	userSvc            *service.UserService
	trackRepo          repository.TrackRepository
	collectRepo        repository.CollectRepository
	trackMapRepo       repository.TrackMapRepository
	companionRepo      repository.CompanionRepository
	analyticsRepo      repository.AnalyticsRepository
	feedbackSvc        *service.FeedbackService
	restrictionSvc     *service.AccountRestrictionService
	routeGroupSvc      *service.TrackRouteGroupService
	trackSubmissionSvc *service.TrackSubmissionService
	screenshotCache    *service.AssetCacheService
	// staticRoot 是服务端本地静态资源根目录（通常为 <LogDir>/static）。
	// 管理后台上传的安装包会落到 <staticRoot>/release/<platform>/ 下，
	// 并通过 /api/v1/static/release/<platform>/<file> 对外下发。
	staticRoot string
}

type adminTrackDeleter interface {
	AdminSoftDeleteAndCleanupTx(ctx context.Context, trackID string) error
}

type trackMapCleanupRepository interface {
	CleanupDeletedTrack(ctx context.Context, trackID string) error
}

type updateTrackBody struct {
	Title    *string `json:"title"`
	CityCode *string `json:"city_code"`
}

// NewHandler 构造管理后台 Handler。
func NewHandler(
	releaseSvc *service.AppReleaseService,
	stsSvc *service.OSSTokenService,
	auth *Authenticator,
	staticRoot string,
	userRepo repository.UserRepository,
	trackRepo repository.TrackRepository,
	collectRepo repository.CollectRepository,
	trackMapRepo repository.TrackMapRepository,
	companionRepo repository.CompanionRepository,
	analyticsRepo repository.AnalyticsRepository,
	userSvc *service.UserService,
	feedbackSvc *service.FeedbackService,
	restrictionSvc *service.AccountRestrictionService,
	routeGroupSvc *service.TrackRouteGroupService,
) *Handler {
	return &Handler{
		releaseSvc:     releaseSvc,
		stsSvc:         stsSvc,
		auth:           auth,
		staticRoot:     staticRoot,
		userRepo:       userRepo,
		userSvc:        userSvc,
		trackRepo:      trackRepo,
		collectRepo:    collectRepo,
		trackMapRepo:   trackMapRepo,
		companionRepo:  companionRepo,
		analyticsRepo:  analyticsRepo,
		feedbackSvc:    feedbackSvc,
		restrictionSvc: restrictionSvc,
		routeGroupSvc:  routeGroupSvc,
	}
}

// SetScreenshotCache injects the shared screenshot cache used to serve track
// screenshots through the admin static proxy.
func (h *Handler) SetScreenshotCache(cache *service.AssetCacheService) {
	if h == nil {
		return
	}
	h.screenshotCache = cache
}

func (h *Handler) SetTrackSubmissionService(svc *service.TrackSubmissionService) {
	if h != nil {
		h.trackSubmissionSvc = svc
	}
}

// ----- 轨迹投稿审核 -----

func (h *Handler) ListTrackSubmissions(ctx context.Context, c *app.RequestContext) {
	if h == nil || h.trackSubmissionSvc == nil {
		c.JSON(http.StatusInternalServerError, utils.H{"error": "track submission service not configured"})
		return
	}
	userID, _ := strconv.ParseInt(strings.TrimSpace(string(c.Query("user_id"))), 10, 64)
	items, err := h.trackSubmissionSvc.ListAdmin(ctx, models.TrackSubmissionListFilter{Status: models.TrackSubmissionStatus(strings.TrimSpace(string(c.Query("status")))), Difficulty: strings.TrimSpace(string(c.Query("difficulty"))), RiskLevel: strings.TrimSpace(string(c.Query("risk_level"))), TrackType: strings.TrimSpace(string(c.Query("track_type"))), UserID: userID, Limit: parseAdminListLimit(string(c.Query("limit")))})
	if err != nil {
		writeAdminTrackSubmissionError(c, err)
		return
	}
	for _, sub := range items {
		rewriteAdminSubmissionImages(sub)
	}
	c.JSON(http.StatusOK, utils.H{"code": 0, "data": utils.H{"items": items, "total": len(items)}})
}

func (h *Handler) GetTrackSubmission(ctx context.Context, c *app.RequestContext) {
	if h == nil || h.trackSubmissionSvc == nil {
		c.JSON(http.StatusInternalServerError, utils.H{"error": "track submission service not configured"})
		return
	}
	selected, err := h.trackSubmissionSvc.GetAdmin(ctx, c.Param("submission_id"))
	if err != nil {
		writeAdminTrackSubmissionError(c, err)
		return
	}
	rewriteAdminSubmissionImages(selected)
	events, err := h.trackSubmissionSvc.Events(ctx, selected.SubmissionID)
	if err != nil {
		writeAdminTrackSubmissionError(c, err)
		return
	}
	var track *models.Track
	if h.trackRepo != nil {
		track, _ = h.trackRepo.FindByID(ctx, selected.TrackID)
		if track != nil {
			h.decorateAdminTrackAssetURLs(ctx, []*models.Track{track})
		}
	}
	c.JSON(http.StatusOK, utils.H{"code": 0, "data": utils.H{"submission": selected, "track": track, "events": events}})
}

func (h *Handler) ReviewTrackSubmission(ctx context.Context, c *app.RequestContext) {
	if h == nil || h.trackSubmissionSvc == nil {
		c.JSON(http.StatusInternalServerError, utils.H{"error": "track submission service not configured"})
		return
	}
	var body struct {
		Decision         string `json:"decision"`
		Reason           string `json:"reason"`
		ExpectedRevision int64  `json:"expected_revision"`
	}
	data, err := c.Body()
	if err != nil || json.Unmarshal(data, &body) != nil || body.ExpectedRevision <= 0 {
		c.JSON(http.StatusBadRequest, utils.H{"error": "invalid payload"})
		return
	}
	reviewer := "admin"
	if h.auth != nil {
		if session := h.auth.SessionFromRequest(c); session != nil && strings.TrimSpace(session.Username) != "" {
			reviewer = session.Username
		}
	}
	sub, err := h.trackSubmissionSvc.Review(ctx, c.Param("submission_id"), body.ExpectedRevision, strings.TrimSpace(body.Decision), reviewer, body.Reason)
	if err != nil {
		writeAdminTrackSubmissionError(c, err)
		return
	}
	c.JSON(http.StatusOK, utils.H{"code": 0, "data": sub})
}

func rewriteAdminSubmissionImages(sub *models.TrackSubmission) {
	if sub == nil {
		return
	}
	for _, image := range sub.Images {
		if image != nil {
			image.URL = rewriteAdminStaticURL(image.URL)
		}
	}
}

func writeAdminTrackSubmissionError(c *app.RequestContext, err error) {
	var invalid *service.InvalidArgumentError
	switch {
	case errors.As(err, &invalid):
		c.JSON(http.StatusBadRequest, utils.H{"error": err.Error()})
	case errors.Is(err, repository.ErrNotFound):
		c.JSON(http.StatusNotFound, utils.H{"error": "not found"})
	case errors.Is(err, repository.ErrAlreadyExists):
		c.JSON(http.StatusConflict, utils.H{"error": "submission revision conflict"})
	default:
		c.JSON(http.StatusInternalServerError, utils.H{"error": err.Error()})
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
	rows := make([]utils.H, 0, len(items))
	for _, u := range items {
		row := utils.H{
			"id":                  u.ID,
			"nickname":            u.Nickname,
			"avatar_url":          u.AvatarURL,
			"signature":           u.Signature,
			"phone":               u.Phone,
			"client_language":     u.ClientLanguage,
			"created_at":          u.CreatedAt,
			"updated_at":          u.UpdatedAt,
			"account_restriction": nil,
		}
		if h.restrictionSvc != nil {
			restriction, err := h.restrictionSvc.FindActive(ctx, u.ID)
			if err == nil {
				row["account_restriction"] = restriction
			} else if !errors.Is(err, repository.ErrNotFound) {
				c.JSON(http.StatusInternalServerError, utils.H{"error": err.Error()})
				return
			}
		}
		rows = append(rows, row)
	}
	total, err := h.userRepo.CountAll(ctx)
	if err != nil {
		c.JSON(http.StatusInternalServerError, utils.H{"error": err.Error()})
		return
	}
	resp := utils.H{
		"items":    rows,
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

type adminAccountRestrictionBody struct {
	Reason    string `json:"reason"`
	ExpiresAt string `json:"expires_at"`
}

func (h *Handler) CreateAccountRestriction(ctx context.Context, c *app.RequestContext) {
	if h == nil || h.restrictionSvc == nil {
		c.JSON(http.StatusInternalServerError, utils.H{"error": "account restriction service not configured"})
		return
	}
	userID, ok := parseAdminUserID(c)
	if !ok {
		return
	}
	var body adminAccountRestrictionBody
	data, err := c.Body()
	if err != nil || json.Unmarshal(data, &body) != nil {
		c.JSON(http.StatusBadRequest, utils.H{"error": "invalid payload"})
		return
	}
	var expiresAt *time.Time
	if strings.TrimSpace(body.ExpiresAt) != "" {
		parsed, err := time.Parse(time.RFC3339, strings.TrimSpace(body.ExpiresAt))
		if err != nil {
			c.JSON(http.StatusBadRequest, utils.H{"error": "expires_at must be RFC3339"})
			return
		}
		expiresAt = &parsed
	}
	item, err := h.restrictionSvc.Create(ctx, service.CreateAccountRestrictionInput{
		UserID:    userID,
		Reason:    body.Reason,
		Operator:  h.adminOperator(c),
		ExpiresAt: expiresAt,
	})
	if err != nil {
		var iae *service.InvalidArgumentError
		if errors.As(err, &iae) {
			c.JSON(http.StatusBadRequest, utils.H{"error": iae.Error()})
			return
		}
		c.JSON(http.StatusInternalServerError, utils.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, utils.H{"code": 0, "data": item})
}

func (h *Handler) GetCurrentAccountRestriction(ctx context.Context, c *app.RequestContext) {
	if h == nil || h.restrictionSvc == nil {
		c.JSON(http.StatusInternalServerError, utils.H{"error": "account restriction service not configured"})
		return
	}
	userID, ok := parseAdminUserID(c)
	if !ok {
		return
	}
	item, err := h.restrictionSvc.FindActive(ctx, userID)
	if err != nil {
		if errors.Is(err, repository.ErrNotFound) {
			c.JSON(http.StatusNotFound, utils.H{"error": "account restriction not found"})
			return
		}
		c.JSON(http.StatusInternalServerError, utils.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, utils.H{"code": 0, "data": item})
}

func (h *Handler) RevokeCurrentAccountRestriction(ctx context.Context, c *app.RequestContext) {
	if h == nil || h.restrictionSvc == nil {
		c.JSON(http.StatusInternalServerError, utils.H{"error": "account restriction service not configured"})
		return
	}
	userID, ok := parseAdminUserID(c)
	if !ok {
		return
	}
	count, err := h.restrictionSvc.RevokeActive(ctx, userID, h.adminOperator(c))
	if err != nil {
		c.JSON(http.StatusInternalServerError, utils.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, utils.H{"code": 0, "data": utils.H{"revoked_count": count}})
}

func parseAdminUserID(c *app.RequestContext) (int64, bool) {
	userID, err := strconv.ParseInt(strings.TrimSpace(c.Param("user_id")), 10, 64)
	if err != nil || userID <= 0 {
		c.JSON(http.StatusBadRequest, utils.H{"error": "invalid user_id"})
		return 0, false
	}
	return userID, true
}

func (h *Handler) adminOperator(c *app.RequestContext) string {
	if h != nil && h.auth != nil {
		if sess := h.auth.SessionFromRequest(c); sess != nil && strings.TrimSpace(sess.Username) != "" {
			return sess.Username
		}
	}
	return "admin"
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
	h.decorateAdminTrackAssetURLs(ctx, items)
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

// UpdateTrack 处理 PUT /admin/api/tracks/:track_id
//
// 管理后台只允许修正运营展示字段：title 与 city_code。
func (h *Handler) UpdateTrack(ctx context.Context, c *app.RequestContext) {
	if h == nil || h.trackRepo == nil {
		c.JSON(http.StatusInternalServerError, utils.H{"error": "track repository not configured"})
		return
	}
	trackID := strings.TrimSpace(c.Param("track_id"))
	if trackID == "" {
		c.JSON(http.StatusBadRequest, utils.H{"error": "track_id is required"})
		return
	}
	var body updateTrackBody
	data, err := c.Body()
	if err != nil || json.Unmarshal(data, &body) != nil {
		c.JSON(http.StatusBadRequest, utils.H{"error": "invalid payload"})
		return
	}
	if body.Title == nil && body.CityCode == nil {
		c.JSON(http.StatusBadRequest, utils.H{"error": "title or city_code is required"})
		return
	}
	track, err := h.trackRepo.FindByID(ctx, trackID)
	if err != nil {
		handleAdminRepoError(c, err)
		return
	}
	oldCityCode := track.CityCode
	if body.Title != nil {
		title := strings.TrimSpace(*body.Title)
		if utf8.RuneCountInString(title) > 128 {
			c.JSON(http.StatusBadRequest, utils.H{"error": "title exceeds 128 characters"})
			return
		}
		track.Title = title
	}
	if body.CityCode != nil {
		cityCode := strings.TrimSpace(*body.CityCode)
		if utf8.RuneCountInString(cityCode) > 16 {
			c.JSON(http.StatusBadRequest, utils.H{"error": "city_code exceeds 16 characters"})
			return
		}
		track.CityCode = cityCode
	}
	if err := h.trackRepo.Update(ctx, track); err != nil {
		handleAdminRepoError(c, err)
		return
	}
	if h.trackMapRepo != nil && body.CityCode != nil && oldCityCode != track.CityCode {
		if index, err := h.trackMapRepo.FindTrackGeoIndex(ctx, track.ID); err == nil && index != nil {
			index.CityCode = track.CityCode
			if err := h.trackMapRepo.UpsertTrackGeoIndex(ctx, index); err != nil {
				c.JSON(http.StatusInternalServerError, utils.H{"error": err.Error()})
				return
			}
		} else if err != nil && !errors.Is(err, repository.ErrNotFound) {
			c.JSON(http.StatusInternalServerError, utils.H{"error": err.Error()})
			return
		}
	}
	c.JSON(http.StatusOK, utils.H{"code": 0, "data": track})
}

// DeleteTrack 处理 DELETE /admin/api/tracks/:track_id
func (h *Handler) DeleteTrack(ctx context.Context, c *app.RequestContext) {
	if h == nil || h.trackRepo == nil {
		c.JSON(http.StatusInternalServerError, utils.H{"error": "track repository not configured"})
		return
	}
	trackID := strings.TrimSpace(c.Param("track_id"))
	if trackID == "" {
		c.JSON(http.StatusBadRequest, utils.H{"error": "track_id is required"})
		return
	}
	if deleter, ok := h.trackRepo.(adminTrackDeleter); ok {
		if err := deleter.AdminSoftDeleteAndCleanupTx(ctx, trackID); err != nil {
			handleAdminRepoError(c, err)
			return
		}
		if h.trackSubmissionSvc != nil {
			if err := h.trackSubmissionSvc.Invalidate(ctx, trackID, "track deleted by admin"); err != nil {
				c.JSON(http.StatusInternalServerError, utils.H{"error": err.Error()})
				return
			}
		}
		c.JSON(http.StatusOK, utils.H{"code": 0, "data": utils.H{"status": "ok"}})
		return
	}

	track, err := h.trackRepo.FindByID(ctx, trackID)
	if err != nil {
		handleAdminRepoError(c, err)
		return
	}
	now := time.Now()
	if track.DeletedAt.IsZero() {
		track.DeletedAt = now
	}
	track.Status = models.TrackStatusDeleted
	track.IsRunning = false
	track.UpdatedAt = now
	if err := h.trackRepo.Update(ctx, track); err != nil {
		handleAdminRepoError(c, err)
		return
	}
	if h.trackSubmissionSvc != nil {
		if err := h.trackSubmissionSvc.Invalidate(ctx, trackID, "track deleted by admin"); err != nil {
			c.JSON(http.StatusInternalServerError, utils.H{"error": err.Error()})
			return
		}
	}
	if h.collectRepo != nil {
		if err := h.collectRepo.RemoveByTrackID(ctx, trackID); err != nil {
			c.JSON(http.StatusInternalServerError, utils.H{"error": err.Error()})
			return
		}
	}
	if cleaner, ok := h.trackMapRepo.(trackMapCleanupRepository); ok {
		if err := cleaner.CleanupDeletedTrack(ctx, trackID); err != nil {
			c.JSON(http.StatusInternalServerError, utils.H{"error": err.Error()})
			return
		}
	}
	c.JSON(http.StatusOK, utils.H{"code": 0, "data": utils.H{"status": "ok"}})
}

func handleAdminRepoError(c *app.RequestContext, err error) {
	var iae *service.InvalidArgumentError
	switch {
	case errors.As(err, &iae):
		c.JSON(http.StatusBadRequest, utils.H{"error": err.Error()})
	case errors.Is(err, repository.ErrNotFound):
		c.JSON(http.StatusNotFound, utils.H{"error": "not found"})
	case errors.Is(err, repository.ErrForbidden), errors.Is(err, service.ErrForbidden):
		c.JSON(http.StatusForbidden, utils.H{"error": "forbidden"})
	default:
		c.JSON(http.StatusInternalServerError, utils.H{"error": err.Error()})
	}
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

func (h *Handler) decorateAdminTrackAssetURLs(ctx context.Context, items []*models.Track) {
	for _, item := range items {
		if item == nil {
			continue
		}
		if h != nil && h.screenshotCache != nil {
			item.TrackScreenshotURL = h.ensureAdminScreenshotCached(ctx, item.UserID, item.ID, item.TrackScreenshotURL)
			item.TrackNoMapBgScreenshotURL = h.ensureAdminScreenshotCached(ctx, item.UserID, item.ID+"_no_map_bg", item.TrackNoMapBgScreenshotURL)
		}
		item.TrackScreenshotURL = rewriteAdminStaticURL(item.TrackScreenshotURL)
		item.TrackNoMapBgScreenshotURL = rewriteAdminStaticURL(item.TrackNoMapBgScreenshotURL)
		item.RawTrackURL = rewriteAdminStaticURL(item.RawTrackURL)
	}
}

func (h *Handler) ensureAdminScreenshotCached(ctx context.Context, userID int64, key, src string) string {
	src = strings.TrimSpace(src)
	if src == "" || h == nil || h.screenshotCache == nil || strings.HasPrefix(src, "/api/v1/static/") || strings.HasPrefix(src, "/admin/api/static/") {
		return src
	}
	cacheCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	if local := h.screenshotCache.EnsureCached(cacheCtx, userID, key, src); local != "" {
		return local
	}
	return src
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
//   - app_version/version: 客户端版本，可空，精确匹配；
//   - phone: 用户手机号，可空，精确匹配；
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
	filter := models.FeedbackListFilter{
		Status:     models.FeedbackStatus(strings.TrimSpace(string(c.Query("status")))),
		AppVersion: strings.TrimSpace(string(c.Query("app_version"))),
		Cursor:     cursor,
	}
	if filter.AppVersion == "" {
		filter.AppVersion = strings.TrimSpace(string(c.Query("version")))
	}
	phone := strings.TrimSpace(string(c.Query("phone")))
	if phone != "" {
		if h.userRepo == nil {
			c.JSON(http.StatusInternalServerError, utils.H{"error": "user repository not configured"})
			return
		}
		user, err := h.userRepo.FindByPhone(ctx, phone)
		if err != nil {
			if errors.Is(err, repository.ErrNotFound) {
				c.JSON(http.StatusOK, utils.H{"code": 0, "data": &models.FeedbackPage{Items: []*models.Feedback{}}})
				return
			}
			c.JSON(http.StatusInternalServerError, utils.H{"error": err.Error()})
			return
		}
		filter.UserID = user.ID
	}
	page, err := h.feedbackSvc.ListOpsFiltered(ctx, filter, parseAdminListLimit(string(c.Query("limit"))))
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

// ----- 路线组运营 -----

// ListRouteGroups 处理 GET /admin/api/route-groups
func (h *Handler) ListRouteGroups(ctx context.Context, c *app.RequestContext) {
	if h == nil || h.routeGroupSvc == nil {
		c.JSON(http.StatusInternalServerError, utils.H{"error": "route group service not configured"})
		return
	}
	filter := models.TrackMapQueryFilter{
		TrackType: strings.TrimSpace(string(c.Query("track_type"))),
		CityCode:  strings.TrimSpace(string(c.Query("city_code"))),
		Limit:     parseAdminListLimit(string(c.Query("limit"))),
	}
	items, err := h.routeGroupSvc.ListRouteGroupSummaries(ctx, filter)
	if err != nil {
		writeAdminRouteGroupError(c, err)
		return
	}
	c.JSON(http.StatusOK, utils.H{"code": 0, "data": utils.H{"items": items, "total": len(items)}})
}

// GetRouteGroup 处理 GET /admin/api/route-groups/:group_id
func (h *Handler) GetRouteGroup(ctx context.Context, c *app.RequestContext) {
	if h == nil || h.routeGroupSvc == nil {
		c.JSON(http.StatusInternalServerError, utils.H{"error": "route group service not configured"})
		return
	}
	detail, err := h.routeGroupSvc.GetRouteGroupDetail(ctx, c.Param("group_id"), parseAdminListLimit(string(c.Query("limit"))))
	if err != nil {
		writeAdminRouteGroupError(c, err)
		return
	}
	if h.trackRepo != nil {
		for _, member := range detail.Members {
			if member == nil || member.Member == nil {
				continue
			}
			track, err := h.trackRepo.FindByID(ctx, member.Member.TrackID)
			if err == nil {
				member.Track = track
			}
		}
	}
	c.JSON(http.StatusOK, utils.H{"code": 0, "data": detail})
}

type renameRouteGroupBody struct {
	Name string `json:"name"`
}

func (h *Handler) RenameRouteGroup(ctx context.Context, c *app.RequestContext) {
	if h == nil || h.routeGroupSvc == nil {
		c.JSON(http.StatusInternalServerError, utils.H{"error": "route group service not configured"})
		return
	}
	var body renameRouteGroupBody
	data, err := c.Body()
	if err != nil || json.Unmarshal(data, &body) != nil {
		c.JSON(http.StatusBadRequest, utils.H{"error": "invalid payload"})
		return
	}
	group, err := h.routeGroupSvc.RenameRouteGroup(ctx, c.Param("group_id"), body.Name)
	if err != nil {
		writeAdminRouteGroupError(c, err)
		return
	}
	c.JSON(http.StatusOK, utils.H{"code": 0, "data": group})
}

func (h *Handler) GetRouteGroupIntroduction(ctx context.Context, c *app.RequestContext) {
	if h == nil || h.routeGroupSvc == nil {
		c.JSON(http.StatusInternalServerError, utils.H{"error": "route group service not configured"})
		return
	}
	introduction, err := h.routeGroupSvc.GetRouteIntroduction(ctx, c.Param("group_id"))
	if errors.Is(err, repository.ErrNotFound) {
		c.JSON(http.StatusOK, utils.H{"code": 0, "data": utils.H{"status": models.TrackRouteIntroductionStatusDraft}})
		return
	}
	if err != nil {
		writeAdminRouteGroupError(c, err)
		return
	}
	c.JSON(http.StatusOK, utils.H{"code": 0, "data": introduction})
}

func (h *Handler) SaveRouteGroupIntroduction(ctx context.Context, c *app.RequestContext) {
	if h == nil || h.routeGroupSvc == nil {
		c.JSON(http.StatusInternalServerError, utils.H{"error": "route group service not configured"})
		return
	}
	var body models.TrackRouteIntroduction
	data, err := c.Body()
	if err != nil || json.Unmarshal(data, &body) != nil {
		c.JSON(http.StatusBadRequest, utils.H{"error": "invalid payload"})
		return
	}
	introduction, err := h.routeGroupSvc.SaveRouteIntroduction(ctx, c.Param("group_id"), &body)
	if err != nil {
		writeAdminRouteGroupError(c, err)
		return
	}
	c.JSON(http.StatusOK, utils.H{"code": 0, "data": introduction})
}

func (h *Handler) PublishRouteGroupIntroduction(ctx context.Context, c *app.RequestContext) {
	h.setRouteGroupIntroductionPublished(ctx, c, true)
}

func (h *Handler) UnpublishRouteGroupIntroduction(ctx context.Context, c *app.RequestContext) {
	h.setRouteGroupIntroductionPublished(ctx, c, false)
}

func (h *Handler) setRouteGroupIntroductionPublished(ctx context.Context, c *app.RequestContext, published bool) {
	if h == nil || h.routeGroupSvc == nil {
		c.JSON(http.StatusInternalServerError, utils.H{"error": "route group service not configured"})
		return
	}
	introduction, err := h.routeGroupSvc.SetRouteIntroductionPublished(ctx, c.Param("group_id"), published)
	if err != nil {
		writeAdminRouteGroupError(c, err)
		return
	}
	c.JSON(http.StatusOK, utils.H{"code": 0, "data": introduction})
}

type mergeRouteGroupBody struct {
	SourceGroupID string `json:"source_group_id"`
}

func (h *Handler) MergeRouteGroup(ctx context.Context, c *app.RequestContext) {
	if h == nil || h.routeGroupSvc == nil {
		c.JSON(http.StatusInternalServerError, utils.H{"error": "route group service not configured"})
		return
	}
	var body mergeRouteGroupBody
	data, err := c.Body()
	if err != nil || json.Unmarshal(data, &body) != nil {
		c.JSON(http.StatusBadRequest, utils.H{"error": "invalid payload"})
		return
	}
	group, err := h.routeGroupSvc.MergeRouteGroups(ctx, c.Param("group_id"), body.SourceGroupID)
	if err != nil {
		writeAdminRouteGroupError(c, err)
		return
	}
	c.JSON(http.StatusOK, utils.H{"code": 0, "data": group})
}

func (h *Handler) RemoveRouteGroupMember(ctx context.Context, c *app.RequestContext) {
	if h == nil || h.routeGroupSvc == nil {
		c.JSON(http.StatusInternalServerError, utils.H{"error": "route group service not configured"})
		return
	}
	group, err := h.routeGroupSvc.RemoveRouteGroupMember(ctx, c.Param("group_id"), c.Param("track_id"))
	if err != nil {
		writeAdminRouteGroupError(c, err)
		return
	}
	c.JSON(http.StatusOK, utils.H{"code": 0, "data": group})
}

type setRepresentativeBody struct {
	TrackID string `json:"track_id"`
}

func (h *Handler) SetRouteGroupRepresentative(ctx context.Context, c *app.RequestContext) {
	if h == nil || h.routeGroupSvc == nil {
		c.JSON(http.StatusInternalServerError, utils.H{"error": "route group service not configured"})
		return
	}
	var body setRepresentativeBody
	data, err := c.Body()
	if err != nil || json.Unmarshal(data, &body) != nil {
		c.JSON(http.StatusBadRequest, utils.H{"error": "invalid payload"})
		return
	}
	group, err := h.routeGroupSvc.SetRepresentativeTrack(ctx, c.Param("group_id"), body.TrackID)
	if err != nil {
		writeAdminRouteGroupError(c, err)
		return
	}
	c.JSON(http.StatusOK, utils.H{"code": 0, "data": group})
}

func writeAdminRouteGroupError(c *app.RequestContext, err error) {
	var iae *service.InvalidArgumentError
	switch {
	case errors.As(err, &iae):
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
