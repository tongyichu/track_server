package admin

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
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
	releaseSvc *service.AppReleaseService
	stsSvc     *service.OSSTokenService
	auth       *Authenticator
	// staticRoot 是服务端本地静态资源根目录（通常为 <LogDir>/static）。
	// 管理后台上传的安装包会落到 <staticRoot>/release/<platform>/ 下，
	// 并通过 /api/v1/static/release/<platform>/<file> 对外下发。
	staticRoot string
}

// NewHandler 构造管理后台 Handler。
func NewHandler(releaseSvc *service.AppReleaseService, stsSvc *service.OSSTokenService, auth *Authenticator, staticRoot string) *Handler {
	return &Handler{releaseSvc: releaseSvc, stsSvc: stsSvc, auth: auth, staticRoot: staticRoot}
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
