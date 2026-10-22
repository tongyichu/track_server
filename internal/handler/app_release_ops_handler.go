package handler

import (
	"context"
	"crypto/md5"
	"crypto/subtle"
	"encoding/hex"
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
	"github.com/tongyichu/track_server/internal/service"
)

const maxOpsAndroidPackageSize int64 = 500 * 1024 * 1024

// AppReleaseOpsHandler 为可信发布工具提供单请求的 Android 安装包发布入口。
type AppReleaseOpsHandler struct {
	svc           *service.AppReleaseService
	staticRoot    string
	internalToken string
}

func NewAppReleaseOpsHandler(svc *service.AppReleaseService, staticRoot, internalToken string) *AppReleaseOpsHandler {
	return &AppReleaseOpsHandler{
		svc:           svc,
		staticRoot:    strings.TrimSpace(staticRoot),
		internalToken: strings.TrimSpace(internalToken),
	}
}

// PublishAndroid 处理 POST /api/v1/ops/releases/android。
//
// 文件落盘与 published 发布记录写入属于同一次调用；发布记录写入失败时会删除本次文件。
func (h *AppReleaseOpsHandler) PublishAndroid(ctx context.Context, c *app.RequestContext) {
	if !h.checkInternalToken(c) {
		return
	}
	if h == nil || h.svc == nil {
		c.JSON(http.StatusInternalServerError, utils.H{"error": "app release service not configured"})
		return
	}
	if h.staticRoot == "" {
		c.JSON(http.StatusServiceUnavailable, utils.H{"error": "release package storage not configured"})
		return
	}

	versionName := strings.TrimSpace(string(c.FormValue("version_name")))
	if versionName == "" {
		c.JSON(http.StatusBadRequest, utils.H{"error": "version_name is required"})
		return
	}
	if len(versionName) > 32 {
		c.JSON(http.StatusBadRequest, utils.H{"error": "version_name is too long"})
		return
	}
	versionCode, ok := parseRequiredPositiveInt64(c, "version_code")
	if !ok {
		return
	}
	minSupportedVersionCode, ok := parseOptionalNonNegativeInt64(c, "min_supported_version_code")
	if !ok {
		return
	}
	if minSupportedVersionCode > versionCode {
		c.JSON(http.StatusBadRequest, utils.H{"error": "min_supported_version_code must be <= version_code"})
		return
	}
	forceUpdate, ok := parseOptionalBool(c, "force_update")
	if !ok {
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
	if fileHeader.Size > maxOpsAndroidPackageSize {
		c.JSON(http.StatusRequestEntityTooLarge, utils.H{
			"error": fmt.Sprintf("file too large: %d bytes, max %d", fileHeader.Size, maxOpsAndroidPackageSize),
		})
		return
	}
	if !strings.EqualFold(filepath.Ext(fileHeader.Filename), ".apk") {
		c.JSON(http.StatusBadRequest, utils.H{"error": "android package must be an .apk file"})
		return
	}

	safeVersionName := sanitizeReleaseFilePart(versionName)
	if safeVersionName == "" {
		safeVersionName = "version"
	}
	finalName := fmt.Sprintf("%d-track-%s-%d.apk", time.Now().UnixMilli(), safeVersionName, versionCode)
	dstDir := filepath.Join(h.staticRoot, "release", string(models.AppReleasePlatformAndroid))
	if err := os.MkdirAll(dstDir, 0o755); err != nil {
		c.JSON(http.StatusInternalServerError, utils.H{"error": "failed to prepare release storage"})
		return
	}
	dstPath := filepath.Join(dstDir, finalName)

	src, err := fileHeader.Open()
	if err != nil {
		c.JSON(http.StatusInternalServerError, utils.H{"error": "failed to open uploaded package"})
		return
	}
	defer func() { _ = src.Close() }()

	dst, err := os.OpenFile(dstPath, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o644)
	if err != nil {
		c.JSON(http.StatusInternalServerError, utils.H{"error": "failed to create release package"})
		return
	}
	hash := md5.New()
	written, copyErr := io.Copy(io.MultiWriter(dst, hash), src)
	closeErr := dst.Close()
	if copyErr != nil || closeErr != nil {
		_ = os.Remove(dstPath)
		c.JSON(http.StatusInternalServerError, utils.H{"error": "failed to store release package"})
		return
	}

	publicURL := "/api/v1/static/release/android/" + finalName
	release, err := h.svc.Publish(ctx, service.PublishReleaseInput{
		Platform:                models.AppReleasePlatformAndroid,
		VersionName:             versionName,
		VersionCode:             versionCode,
		MinSupportedVersionCode: minSupportedVersionCode,
		PackageURL:              publicURL,
		PackageSize:             written,
		PackageMD5:              hex.EncodeToString(hash.Sum(nil)),
		ReleaseNotes:            string(c.FormValue("release_notes")),
		ForceUpdate:             forceUpdate,
		Status:                  models.AppReleaseStatusPublished,
		OperatorName:            "android-track-release",
	})
	if err != nil {
		_ = os.Remove(dstPath)
		var invalid *service.InvalidArgumentError
		if errors.As(err, &invalid) {
			c.JSON(http.StatusBadRequest, utils.H{"error": invalid.Error()})
			return
		}
		c.JSON(http.StatusInternalServerError, utils.H{"error": "failed to publish app release"})
		return
	}

	c.JSON(http.StatusOK, successResponse(release))
}

func (h *AppReleaseOpsHandler) checkInternalToken(c *app.RequestContext) bool {
	if h == nil || h.internalToken == "" {
		c.JSON(http.StatusServiceUnavailable, utils.H{"error": "ops internal auth not configured"})
		return false
	}
	token := strings.TrimSpace(string(c.Request.Header.Peek("X-Internal-Token")))
	if token == "" {
		c.JSON(http.StatusUnauthorized, utils.H{"error": "missing internal token"})
		return false
	}
	if len(token) != len(h.internalToken) || subtle.ConstantTimeCompare([]byte(token), []byte(h.internalToken)) != 1 {
		c.JSON(http.StatusForbidden, utils.H{"error": "forbidden"})
		return false
	}
	return true
}

func parseRequiredPositiveInt64(c *app.RequestContext, field string) (int64, bool) {
	raw := strings.TrimSpace(string(c.FormValue(field)))
	value, err := strconv.ParseInt(raw, 10, 64)
	if err != nil || value <= 0 {
		c.JSON(http.StatusBadRequest, utils.H{"error": field + " must be a positive integer"})
		return 0, false
	}
	return value, true
}

func parseOptionalNonNegativeInt64(c *app.RequestContext, field string) (int64, bool) {
	raw := strings.TrimSpace(string(c.FormValue(field)))
	if raw == "" {
		return 0, true
	}
	value, err := strconv.ParseInt(raw, 10, 64)
	if err != nil || value < 0 {
		c.JSON(http.StatusBadRequest, utils.H{"error": field + " must be a non-negative integer"})
		return 0, false
	}
	return value, true
}

func parseOptionalBool(c *app.RequestContext, field string) (bool, bool) {
	raw := strings.TrimSpace(string(c.FormValue(field)))
	if raw == "" {
		return false, true
	}
	value, err := strconv.ParseBool(raw)
	if err != nil {
		c.JSON(http.StatusBadRequest, utils.H{"error": field + " must be a boolean"})
		return false, false
	}
	return value, true
}

func sanitizeReleaseFilePart(value string) string {
	var b strings.Builder
	for _, r := range value {
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
	return strings.Trim(b.String(), "._-")
}
