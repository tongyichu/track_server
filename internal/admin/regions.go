package admin

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"

	"github.com/cloudwego/hertz/pkg/app"
	"github.com/cloudwego/hertz/pkg/common/utils"
	"github.com/tongyichu/track_server/internal/regions"
)

func (h *Handler) SetRegionService(svc *regions.Service) { h.regionSvc = svc }
func (h *Handler) SetRegionResearcher(researcher *regions.Researcher) {
	h.regionResearcher = researcher
}

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
	photo, err := h.saveRegionPhoto(src, regions.Photo{Title: file.Filename, Provider: "manual"})
	if err != nil {
		c.JSON(http.StatusBadRequest, utils.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusCreated, utils.H{"code": 0, "data": utils.H{"photo": photo, "cover_url": photo.URL}})
}
