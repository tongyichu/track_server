package admin

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/hex"
	"image"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"unicode/utf8"

	"github.com/cloudwego/hertz/pkg/app"
	"github.com/cloudwego/hertz/pkg/common/utils"
	"github.com/tongyichu/track_server/internal/regions"
)

func (h *Handler) UploadRegionPhoto(ctx context.Context, c *app.RequestContext) {
	if h.regionSvc == nil || strings.TrimSpace(h.staticRoot) == "" {
		c.JSON(http.StatusServiceUnavailable, utils.H{"error": "地区图片服务暂不可用"})
		return
	}
	catalog, err := h.regionSvc.Read(ctx)
	if err != nil {
		c.JSON(http.StatusServiceUnavailable, utils.H{"error": "地区目录读取失败"})
		return
	}
	var node *regions.Node
	for i := range catalog.Nodes {
		if catalog.Nodes[i].ID == c.Param("node_id") {
			node = &catalog.Nodes[i]
			break
		}
	}
	if node == nil {
		c.JSON(http.StatusNotFound, utils.H{"error": "请先保存当前地区"})
		return
	}
	if node.Level != "province" && node.Level != "city" || node.RegionID == "" {
		c.JSON(http.StatusBadRequest, utils.H{"error": "仅省份和城市支持录入图片"})
		return
	}
	caption := strings.TrimSpace(string(c.FormValue("caption")))
	if utf8.RuneCountInString(caption) > 500 {
		c.JSON(http.StatusBadRequest, utils.H{"error": "图片说明不能超过500字"})
		return
	}
	file, err := c.FormFile("file")
	if err != nil || file == nil {
		c.JSON(http.StatusBadRequest, utils.H{"error": "请选择图片"})
		return
	}
	if file.Size <= 0 || file.Size > maxScenicSpotPhotoSize {
		c.JSON(http.StatusRequestEntityTooLarge, utils.H{"error": "图片不能超过8MiB"})
		return
	}
	src, err := file.Open()
	if err != nil {
		c.JSON(http.StatusBadRequest, utils.H{"error": "图片读取失败"})
		return
	}
	defer src.Close()
	raw, err := io.ReadAll(io.LimitReader(src, maxScenicSpotPhotoSize+1))
	if err != nil || len(raw) > maxScenicSpotPhotoSize {
		c.JSON(http.StatusBadRequest, utils.H{"error": "图片大小无效"})
		return
	}
	config, format, err := image.DecodeConfig(bytes.NewReader(raw))
	if err != nil || config.Width <= 0 || config.Height <= 0 || config.Width > 12000 || config.Height > 12000 || (format != "jpeg" && format != "png") {
		c.JSON(http.StatusBadRequest, utils.H{"error": "仅支持12000×12000以内的JPG或PNG图片"})
		return
	}
	var id [12]byte
	if _, err := rand.Read(id[:]); err != nil {
		c.JSON(http.StatusInternalServerError, utils.H{"error": "图片ID生成失败"})
		return
	}
	photoID := hex.EncodeToString(id[:])
	ext := ".jpg"
	if format == "png" {
		ext = ".png"
	}
	rel := filepath.Join("regions", node.RegionID, photoID+ext)
	dst := filepath.Join(h.staticRoot, rel)
	if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
		c.JSON(http.StatusInternalServerError, utils.H{"error": "图片存储暂不可用"})
		return
	}
	if err := os.WriteFile(dst, raw, 0o644); err != nil {
		c.JSON(http.StatusInternalServerError, utils.H{"error": "图片保存失败"})
		return
	}
	// Uploading an asset does not commit an editor's unfinished node draft.
	photo := regions.Photo{ID: photoID, URL: "/api/v1/static/" + filepath.ToSlash(rel), Caption: caption}
	c.JSON(http.StatusCreated, utils.H{"code": 0, "data": utils.H{"photo": photo}})
}
