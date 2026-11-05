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

func (h *Handler) SaveRegionNode(ctx context.Context, c *app.RequestContext) {
	if h.regionSvc == nil {
		c.JSON(http.StatusServiceUnavailable, utils.H{"error": "地区目录暂不可用"})
		return
	}
	var input struct {
		BaseNode *regions.Node `json:"base_node"`
		Node     regions.Node  `json:"node"`
	}
	body, err := c.Body()
	if err != nil || len(body) > 128<<10 || json.Unmarshal(body, &input) != nil || input.Node.ID == "" {
		c.JSON(http.StatusBadRequest, utils.H{"error": "地区节点格式无效"})
		return
	}
	catalog, err := h.regionSvc.SaveNode(ctx, input.BaseNode, input.Node)
	if errors.Is(err, regions.ErrNodeConflict) {
		c.JSON(http.StatusConflict, utils.H{"error": "该地区已被其他管理员修改，请刷新页面后核对"})
		return
	}
	if errors.Is(err, regions.ErrVersionConflict) {
		c.JSON(http.StatusConflict, utils.H{"error": "地区目录正在更新，请重试保存"})
		return
	}
	if err != nil {
		c.JSON(http.StatusBadRequest, utils.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, utils.H{"code": 0, "data": catalog})
}

func (h *Handler) DeleteRegionCountry(ctx context.Context, c *app.RequestContext) {
	if h.regionSvc == nil {
		c.JSON(http.StatusServiceUnavailable, utils.H{"error": "地区目录暂不可用"})
		return
	}
	var input struct {
		ExpectedVersion int64 `json:"expected_version"`
	}
	body, err := c.Body()
	if err != nil || len(body) > 1024 || json.Unmarshal(body, &input) != nil || input.ExpectedVersion <= 0 {
		c.JSON(http.StatusBadRequest, utils.H{"error": "删除请求格式无效"})
		return
	}
	catalog, err := h.regionSvc.DeleteCountry(ctx, c.Param("node_id"), input.ExpectedVersion)
	if errors.Is(err, regions.ErrVersionConflict) || errors.Is(err, regions.ErrNodeConflict) {
		c.JSON(http.StatusConflict, utils.H{"error": "地区目录已更新，请刷新页面后核对再删除"})
		return
	}
	if err != nil {
		c.JSON(http.StatusBadRequest, utils.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, utils.H{"code": 0, "data": catalog})
}
