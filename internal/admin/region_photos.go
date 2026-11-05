package admin

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"image"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"unicode/utf8"

	"github.com/cloudwego/hertz/pkg/app"
	"github.com/cloudwego/hertz/pkg/common/utils"
	"github.com/tongyichu/track_server/internal/regions"
	_ "golang.org/x/image/webp"
)

func (h *Handler) SaveRegionPhotos(ctx context.Context, c *app.RequestContext) {
	if h.regionSvc == nil {
		c.JSON(http.StatusServiceUnavailable, utils.H{"error": "地区图片服务暂不可用"})
		return
	}
	var input struct {
		Photos     *[]regions.Photo `json:"photos"`
		BasePhotos *[]regions.Photo `json:"base_photos"`
	}
	body := c.Request.Body()
	if len(body) > 128<<10 || json.Unmarshal(body, &input) != nil || input.Photos == nil || input.BasePhotos == nil {
		c.JSON(http.StatusBadRequest, utils.H{"error": "photos和base_photos须为图片数组"})
		return
	}
	node, version, err := h.regionSvc.SavePhotos(ctx, c.Param("node_id"), *input.BasePhotos, *input.Photos)
	if errors.Is(err, regions.ErrNodeConflict) || errors.Is(err, regions.ErrVersionConflict) {
		c.JSON(http.StatusConflict, utils.H{"error": "图片已被其他操作修改，请重新打开地区后核对"})
		return
	}
	if err != nil {
		c.JSON(http.StatusBadRequest, utils.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, utils.H{"code": 0, "data": utils.H{"node": node, "version": version}})
}

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
	if err != nil || config.Width <= 0 || config.Height <= 0 || config.Width > 12000 || config.Height > 12000 || (format != "jpeg" && format != "png" && format != "webp") {
		c.JSON(http.StatusBadRequest, utils.H{"error": "仅支持12000×12000以内的JPG、PNG或WebP图片"})
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
	} else if format == "webp" {
		ext = ".webp"
	}
	photoURL, err := h.storeRegionPhoto(node.RegionID, photoID, ext, raw)
	if err != nil {
		c.JSON(http.StatusInternalServerError, utils.H{"error": "图片保存失败"})
		return
	}
	// Uploading an asset does not commit an editor's unfinished node draft.
	photo := regions.Photo{ID: photoID, URL: photoURL, Caption: caption}
	c.JSON(http.StatusCreated, utils.H{"code": 0, "data": utils.H{"photo": photo, "image_info": photoImageInfo{Width: config.Width, Height: config.Height, SizeBytes: int64(len(raw))}}})
}

func (h *Handler) storeRegionPhoto(regionID, photoID, ext string, raw []byte) (string, error) {
	rel := filepath.Join("regions", regionID, photoID+ext)
	dst := filepath.Join(h.staticRoot, rel)
	if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
		return "", err
	}
	tmp, err := os.CreateTemp(filepath.Dir(dst), ".photo-*")
	if err != nil {
		return "", err
	}
	defer os.Remove(tmp.Name())
	_, writeErr := tmp.Write(raw)
	closeErr := tmp.Close()
	if writeErr != nil {
		return "", writeErr
	}
	if closeErr != nil {
		return "", closeErr
	}
	if err := os.Chmod(tmp.Name(), 0o644); err != nil {
		return "", err
	}
	if err := os.Rename(tmp.Name(), dst); err != nil {
		return "", err
	}
	return "/api/v1/static/" + filepath.ToSlash(rel), nil
}

func (h *Handler) SetPixabayClient(client *regions.PixabayClient) { h.pixabay = client }

func pixabayError(c *app.RequestContext, err error) {
	status := http.StatusBadGateway
	switch {
	case errors.Is(err, regions.ErrPixabayUnavailable):
		status = http.StatusServiceUnavailable
	case errors.Is(err, regions.ErrPixabayBusy):
		status = http.StatusTooManyRequests
	case errors.Is(err, regions.ErrPixabayExpired):
		status = http.StatusGone
	}
	c.JSON(status, utils.H{"error": err.Error()})
}

func (h *Handler) SearchPixabayPhotos(ctx context.Context, c *app.RequestContext) {
	var input struct {
		Query string `json:"query"`
		Page  int    `json:"page"`
	}
	if err := c.BindJSON(&input); err != nil || strings.TrimSpace(input.Query) == "" || utf8.RuneCountInString(input.Query) > 100 || input.Page < 1 || input.Page > 167 {
		c.JSON(http.StatusBadRequest, utils.H{"error": "请填写100字以内的查询关键词和有效页码"})
		return
	}
	result, err := h.pixabay.Search(ctx, input.Query, input.Page)
	if err != nil {
		pixabayError(c, err)
		return
	}
	c.JSON(http.StatusOK, utils.H{"code": 0, "data": result})
}

func (h *Handler) ImportPixabayPhoto(ctx context.Context, c *app.RequestContext) {
	if h.regionSvc == nil || strings.TrimSpace(h.staticRoot) == "" {
		c.JSON(http.StatusServiceUnavailable, utils.H{"error": "地区图片服务暂不可用"})
		return
	}
	var input struct {
		Query string `json:"query"`
		Page  int    `json:"page"`
		ID    string `json:"id"`
	}
	if err := c.BindJSON(&input); err != nil || input.ID == "" || len(input.ID) > 20 {
		c.JSON(http.StatusBadRequest, utils.H{"error": "请选择查询结果中的图片"})
		return
	}
	if id, err := strconv.ParseInt(input.ID, 10, 64); err != nil || id <= 0 || strconv.FormatInt(id, 10) != input.ID {
		c.JSON(http.StatusBadRequest, utils.H{"error": "候选图片ID无效"})
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
	if (node.Level != "province" && node.Level != "city") || node.RegionID == "" {
		c.JSON(http.StatusBadRequest, utils.H{"error": "仅省份和城市支持导入图片"})
		return
	}
	photoID := "pixabay-" + input.ID
	for _, photo := range node.Photos {
		if photo.ID == photoID && photo.Provider == "pixabay" && strings.HasPrefix(photo.URL, "/api/v1/static/regions/"+node.RegionID+"/") {
			c.JSON(http.StatusOK, utils.H{"code": 0, "data": utils.H{"photo": photo}})
			return
		}
	}
	raw, format, candidate, err := h.pixabay.Download(ctx, input.Query, input.Page, input.ID)
	if err != nil {
		pixabayError(c, err)
		return
	}
	ext := ".jpg"
	if format == "png" {
		ext = ".png"
	}
	photoURL, err := h.storeRegionPhoto(node.RegionID, photoID, ext, raw)
	if err != nil {
		c.JSON(http.StatusInternalServerError, utils.H{"error": "图片保存失败"})
		return
	}
	photo := regions.Photo{ID: photoID, URL: photoURL, AltText: candidate.Tags, Attribution: candidate.Author,
		SourceURL: candidate.SourceURL, Provider: "pixabay", License: "Pixabay Content License", LicenseURL: "https://pixabay.com/service/license-summary/"}
	c.JSON(http.StatusCreated, utils.H{"code": 0, "data": utils.H{"photo": photo, "image_info": photoInfoFromBytes(raw)}})
}
