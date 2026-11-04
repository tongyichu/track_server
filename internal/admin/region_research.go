package admin

import (
	"context"
	"net/http"

	"github.com/cloudwego/hertz/pkg/app"
	"github.com/cloudwego/hertz/pkg/common/utils"
	"github.com/tongyichu/track_server/internal/regions"
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
