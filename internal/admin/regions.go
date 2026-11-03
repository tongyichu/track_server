package admin

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"image"
	_ "image/jpeg"
	_ "image/png"
	"io"
	"net/http"
	"os"
	"path/filepath"

	"github.com/cloudwego/hertz/pkg/app"
	"github.com/cloudwego/hertz/pkg/common/utils"
	"github.com/tongyichu/track_server/internal/regions"
)

func (h *Handler) SetRegionService(svc *regions.Service) { h.regionSvc = svc }

func (h *Handler) GetRegionCatalog(ctx context.Context, c *app.RequestContext) {
	if h.regionSvc == nil {
		c.JSON(http.StatusServiceUnavailable, utils.H{"error": "地区目录暂不可用"})
		return
	}
	catalog, err := h.regionSvc.Read(ctx)
	if err != nil {
		c.JSON(http.StatusServiceUnavailable, utils.H{"error": "地区目录读取失败"})
		return
	}
	c.Header("Cache-Control", "no-store")
	c.JSON(http.StatusOK, utils.H{"code": 0, "data": catalog})
}

func (h *Handler) SaveRegionCatalog(ctx context.Context, c *app.RequestContext) {
	if h.regionSvc == nil {
		c.JSON(http.StatusServiceUnavailable, utils.H{"error": "地区目录暂不可用"})
		return
	}
	var input regions.Catalog
	body, err := c.Body()
	if err != nil || len(body) > 4<<20 || json.Unmarshal(body, &input) != nil {
		c.JSON(http.StatusBadRequest, utils.H{"error": "地区目录格式无效"})
		return
	}
	catalog, err := h.regionSvc.Save(ctx, input.Version, input.Nodes)
	if errors.Is(err, regions.ErrVersionConflict) {
		c.JSON(http.StatusConflict, utils.H{"error": "地区目录已被其他管理员更新，请重新加载"})
		return
	}
	if err != nil {
		c.JSON(http.StatusBadRequest, utils.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, utils.H{"code": 0, "data": catalog})
}

func (h *Handler) UploadRegionImage(_ context.Context, c *app.RequestContext) {
	if h.staticRoot == "" {
		c.JSON(http.StatusServiceUnavailable, utils.H{"error": "图片存储暂不可用"})
		return
	}
	file, err := c.FormFile("image")
	if err != nil || file.Size <= 0 || file.Size > 8<<20 {
		c.JSON(http.StatusBadRequest, utils.H{"error": "请选择不超过 8 MiB 的图片"})
		return
	}
	src, err := file.Open()
	if err != nil {
		c.JSON(http.StatusBadRequest, utils.H{"error": "无法读取图片"})
		return
	}
	defer src.Close()
	raw, err := io.ReadAll(io.LimitReader(src, (8<<20)+1))
	if err != nil || len(raw) == 0 || len(raw) > 8<<20 {
		c.JSON(http.StatusBadRequest, utils.H{"error": "图片大小无效"})
		return
	}
	info, format, err := image.DecodeConfig(bytes.NewReader(raw))
	if err != nil || info.Width <= 0 || info.Height <= 0 || info.Width > 12000 || info.Height > 12000 || (format != "jpeg" && format != "png") {
		c.JSON(http.StatusBadRequest, utils.H{"error": "仅支持有效的 JPEG 或 PNG 图片"})
		return
	}
	var random [16]byte
	if _, err := rand.Read(random[:]); err != nil {
		c.JSON(http.StatusInternalServerError, utils.H{"error": "生成图片名称失败"})
		return
	}
	ext := ".jpg"
	if format == "png" {
		ext = ".png"
	}
	name := hex.EncodeToString(random[:]) + ext
	dir := filepath.Join(h.staticRoot, "regions")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		c.JSON(http.StatusInternalServerError, utils.H{"error": "图片目录不可用"})
		return
	}
	if err := os.WriteFile(filepath.Join(dir, name), raw, 0o644); err != nil {
		c.JSON(http.StatusInternalServerError, utils.H{"error": "保存图片失败"})
		return
	}
	c.JSON(http.StatusCreated, utils.H{"code": 0, "data": utils.H{"cover_url": "/api/v1/static/regions/" + name}})
}
