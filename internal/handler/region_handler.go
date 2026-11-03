package handler

import (
	"context"
	"net/http"
	"strconv"

	"github.com/cloudwego/hertz/pkg/app"
	"github.com/cloudwego/hertz/pkg/common/utils"
	"github.com/tongyichu/track_server/internal/regions"
)

type RegionHandler struct{ service *regions.Service }

func NewRegionHandler(service *regions.Service) *RegionHandler {
	return &RegionHandler{service: service}
}

func (h *RegionHandler) GetCatalog(ctx context.Context, c *app.RequestContext) {
	if h.service == nil {
		c.JSON(http.StatusServiceUnavailable, utils.H{"error": "region catalog unavailable"})
		return
	}
	catalog, err := h.service.Read(ctx)
	if err != nil {
		c.JSON(http.StatusServiceUnavailable, utils.H{"error": "region catalog unavailable"})
		return
	}
	known, err := strconv.ParseInt(string(c.Query("known_version")), 10, 64)
	if err == nil && known == catalog.Version {
		c.JSON(http.StatusOK, utils.H{"code": 0, "data": utils.H{"version": catalog.Version, "unchanged": true}})
		return
	}
	c.JSON(http.StatusOK, utils.H{"code": 0, "data": catalog})
}
