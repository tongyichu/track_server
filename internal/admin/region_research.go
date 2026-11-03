package admin

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"image"
	"image/color"
	"image/jpeg"
	_ "image/png"
	"io"
	"math"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/cloudwego/hertz/pkg/app"
	"github.com/cloudwego/hertz/pkg/common/utils"
	"github.com/tongyichu/track_server/internal/regions"
	"golang.org/x/image/draw"
)

func (h *Handler) ResearchRegion(ctx context.Context, c *app.RequestContext) {
	if h.regionSvc == nil || h.regionResearcher == nil || !h.regionResearcher.Ready() {
		c.JSON(http.StatusServiceUnavailable, utils.H{"error": "城市 AI 联网查询暂不可用"})
		return
	}
	var input struct {
		ID string `json:"id"`
	}
	if err := c.BindJSON(&input); err != nil || input.ID == "" {
		c.JSON(http.StatusBadRequest, utils.H{"error": "请选择已保存的城市节点"})
		return
	}
	catalog, err := h.regionSvc.Read(ctx)
	if err != nil {
		c.JSON(http.StatusServiceUnavailable, utils.H{"error": "地区目录读取失败"})
		return
	}
	byID := make(map[string]regions.Node, len(catalog.Nodes))
	for _, node := range catalog.Nodes {
		byID[node.ID] = node
	}
	node, ok := byID[input.ID]
	if !ok || node.RegionID == "" {
		c.JSON(http.StatusNotFound, utils.H{"error": "城市节点尚未保存"})
		return
	}
	path := []string{node.Name}
	seen := map[string]bool{node.ID: true}
	for parentID := node.ParentID; parentID != "" && len(path) < 12; {
		parent, exists := byID[parentID]
		if !exists || seen[parentID] {
			break
		}
		seen[parentID] = true
		path = append([]string{parent.Name}, path...)
		parentID = parent.ParentID
	}
	draft, err := h.regionResearcher.Research(ctx, node, path)
	if err != nil {
		c.JSON(http.StatusBadGateway, utils.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, utils.H{"code": 0, "data": draft})
}

func (h *Handler) ImportRegionImage(ctx context.Context, c *app.RequestContext) {
	if h.staticRoot == "" || h.regionResearcher == nil {
		c.JSON(http.StatusServiceUnavailable, utils.H{"error": "图片导入暂不可用"})
		return
	}
	var input struct {
		FileTitle string `json:"file_title"`
	}
	if err := c.BindJSON(&input); err != nil || input.FileTitle == "" {
		c.JSON(http.StatusBadRequest, utils.H{"error": "图片来源无效"})
		return
	}
	candidate, err := h.regionResearcher.CommonsPhoto(ctx, input.FileTitle)
	if err != nil {
		c.JSON(http.StatusBadRequest, utils.H{"error": err.Error()})
		return
	}
	client := &http.Client{Timeout: 30 * time.Second, CheckRedirect: func(req *http.Request, _ []*http.Request) error {
		if !isWikimediaUploadURL(req.URL) {
			return errors.New("图片跳转到非 Wikimedia 来源")
		}
		return nil
	}}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, candidate.PreviewURL, nil)
	if err != nil {
		c.JSON(http.StatusBadRequest, utils.H{"error": "图片地址无效"})
		return
	}
	resp, err := client.Do(req)
	if err != nil {
		c.JSON(http.StatusBadGateway, utils.H{"error": "下载图片失败"})
		return
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		c.JSON(http.StatusBadGateway, utils.H{"error": "图片来源暂不可用"})
		return
	}
	photo, err := h.saveRegionPhoto(io.LimitReader(resp.Body, (12<<20)+1), regions.Photo{
		Title: candidate.FileTitle, SourceURL: candidate.SourceURL, Attribution: candidate.Attribution,
		License: candidate.License, LicenseURL: candidate.LicenseURL, Provider: "wikimedia_commons",
	})
	if err != nil {
		c.JSON(http.StatusBadGateway, utils.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusCreated, utils.H{"code": 0, "data": utils.H{"photo": photo}})
}

func isWikimediaUploadURL(u *url.URL) bool {
	return u.Scheme == "https" && u.Hostname() == "upload.wikimedia.org" && u.User == nil && strings.HasPrefix(u.Path, "/wikipedia/commons/")
}

func (h *Handler) saveRegionPhoto(src io.Reader, photo regions.Photo) (regions.Photo, error) {
	raw, err := io.ReadAll(io.LimitReader(src, (12<<20)+1))
	if err != nil || len(raw) == 0 || len(raw) > 12<<20 {
		return photo, errors.New("图片大小无效")
	}
	info, format, err := image.DecodeConfig(bytes.NewReader(raw))
	if err != nil || (format != "jpeg" && format != "png") || info.Width < 1 || info.Height < 1 || info.Width > 12000 || info.Height > 12000 || int64(info.Width)*int64(info.Height) > 40_000_000 {
		return photo, errors.New("仅支持有效的 JPEG 或 PNG 图片，像素不得超过 4000 万")
	}
	decoded, _, err := image.Decode(bytes.NewReader(raw))
	if err != nil {
		return photo, errors.New("图片解码失败")
	}
	scale := math.Min(1, math.Min(1600/float64(info.Width), 1600/float64(info.Height)))
	w, height := int(math.Round(float64(info.Width)*scale)), int(math.Round(float64(info.Height)*scale))
	if w < 1 {
		w = 1
	}
	if height < 1 {
		height = 1
	}
	canvas := image.NewRGBA(image.Rect(0, 0, w, height))
	draw.Draw(canvas, canvas.Bounds(), image.NewUniform(color.White), image.Point{}, draw.Src)
	draw.ApproxBiLinear.Scale(canvas, canvas.Bounds(), decoded, decoded.Bounds(), draw.Over, nil)
	var encoded bytes.Buffer
	if err := jpeg.Encode(&encoded, canvas, &jpeg.Options{Quality: 82}); err != nil {
		return photo, errors.New("图片压缩失败")
	}
	var random [16]byte
	if _, err := rand.Read(random[:]); err != nil {
		return photo, errors.New("生成图片名称失败")
	}
	name := hex.EncodeToString(random[:]) + ".jpg"
	dir := filepath.Join(h.staticRoot, "regions")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return photo, errors.New("图片目录不可用")
	}
	if err := os.WriteFile(filepath.Join(dir, name), encoded.Bytes(), 0o644); err != nil {
		return photo, errors.New("保存图片失败")
	}
	photo.ID = strings.TrimSuffix(name, ".jpg")
	photo.URL = "/api/v1/static/regions/" + name
	photo.Title = truncateRegionPhotoText(photo.Title, 300)
	photo.Attribution = truncateRegionPhotoText(photo.Attribution, 500)
	photo.License = truncateRegionPhotoText(photo.License, 100)
	return photo, nil
}

func truncateRegionPhotoText(value string, limit int) string {
	for len(value) > limit {
		_, size := utf8.DecodeLastRuneInString(value)
		value = value[:len(value)-size]
	}
	return value
}
