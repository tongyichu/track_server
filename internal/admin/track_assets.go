package admin

import (
	"context"
	"crypto/sha256"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/cloudwego/hertz/pkg/app"
	"github.com/cloudwego/hertz/pkg/common/utils"
	"github.com/tongyichu/track_server/internal/service"
)

func (h *Handler) SetTrackAssetCaches(raw, preview *service.AssetCacheService) {
	h.rawTrackCache, h.trackPreviewCache = raw, preview
}

// GetTrackAsset resolves a persisted asset under the tracks read permission.
// URLs are never accepted from the browser, and downloads do not rewrite track fields.
func (h *Handler) GetTrackAsset(ctx context.Context, c *app.RequestContext) {
	if h == nil || h.trackRepo == nil {
		c.JSON(http.StatusServiceUnavailable, utils.H{"error": "轨迹服务未配置"})
		return
	}
	track, err := h.trackRepo.FindByID(ctx, c.Param("track_id"))
	if err != nil {
		handleAdminRepoError(c, err)
		return
	}
	asset := c.Param("asset")
	var source string
	var cache *service.AssetCacheService
	switch asset {
	case "raw":
		source, cache = track.RawTrackURL, h.rawTrackCache
	case "legacy":
		source, cache = track.TrackScreenshotURL, h.screenshotCache
	case "no_map":
		source, cache = track.TrackNoMapBgScreenshotURL, h.screenshotCache
	case "square_day", "square_night", "landscape_day", "landscape_night":
		source, cache = track.PreviewImages.Variant(asset), h.trackPreviewCache
	default:
		c.JSON(http.StatusBadRequest, utils.H{"error": "无效的轨迹资源类型"})
		return
	}
	source = strings.TrimSpace(source)
	if source == "" {
		c.JSON(http.StatusNotFound, utils.H{"error": "轨迹资源缺失"})
		return
	}
	var file string
	for _, prefix := range []string{"/api/v1/static/", "/admin/api/static/"} {
		if !strings.HasPrefix(source, prefix) {
			continue
		}
		rel, valid := cleanAdminStaticPath(strings.TrimPrefix(source, prefix))
		allowed := strings.HasPrefix(rel, "screenshots/") || strings.HasPrefix(rel, "track_previews/")
		if asset == "raw" {
			allowed = strings.HasPrefix(rel, "raw_tracks/")
		}
		if !valid || !allowed || h.staticRoot == "" {
			c.JSON(http.StatusUnprocessableEntity, utils.H{"error": "轨迹资源路径无效"})
			return
		}
		file = filepath.Join(h.staticRoot, filepath.FromSlash(rel))
		break
	}
	if file == "" {
		if cache == nil {
			c.JSON(http.StatusServiceUnavailable, utils.H{"error": "轨迹资源缓存未配置"})
			return
		}
		// Include the source version so changing a URL cannot return an old cached asset.
		version := sha256.Sum256([]byte(source))
		key := fmt.Sprintf("%s_admin_%s_%x", track.ID, asset, version[:8])
		if cache == h.trackPreviewCache {
			key = fmt.Sprintf("%s_preview_%s_%x", track.ID, asset, version[:8])
		}
		cacheCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
		defer cancel()
		file, err = cache.EnsureCachedFile(cacheCtx, track.UserID, key, source)
		if err != nil {
			c.JSON(http.StatusBadGateway, utils.H{"error": "轨迹资源读取失败，请重试"})
			return
		}
	}
	info, err := os.Stat(file)
	if err != nil || info.IsDir() {
		c.JSON(http.StatusNotFound, utils.H{"error": "轨迹资源文件不存在"})
		return
	}
	c.Header("Cache-Control", "no-store")
	c.Header("X-Content-Type-Options", "nosniff")
	c.Header("Content-Security-Policy", "sandbox")
	requestURI := string(c.Request.RequestURI())
	defer c.Request.SetRequestURI(requestURI)
	if asset == "raw" {
		ext := filepath.Ext(file)
		if parsed, err := url.Parse(source); err == nil {
			switch original := strings.ToLower(filepath.Ext(parsed.Path)); original {
			case ".dat", ".json", ".geojson", ".gpx", ".kml", ".kmz", ".zip":
				ext = original
			}
		}
		c.FileAttachment(file, track.ID+ext)
		return
	}
	c.File(file)
}
