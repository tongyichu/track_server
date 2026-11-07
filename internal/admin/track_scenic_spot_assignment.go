package admin

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"

	"github.com/cloudwego/hertz/pkg/app"
	"github.com/cloudwego/hertz/pkg/common/utils"
	"github.com/tongyichu/track_server/internal/models"
	"github.com/tongyichu/track_server/internal/service"
)

func (h *Handler) AssignTrackMapScenicSpot(ctx context.Context, c *app.RequestContext) {
	if !h.scenicSpotServiceReady(c) {
		return
	}
	var input service.TrackMapScenicSpotInput
	if err := json.Unmarshal(c.Request.Body(), &input); err != nil {
		c.JSON(http.StatusBadRequest, utils.H{"error": "invalid payload"})
		return
	}
	spot, err := h.scenicSpotSvc.AssignTrackMapScenicSpot(ctx, h.adminOperator(c), strings.TrimSpace(c.Param("track_id")), input)
	if err != nil {
		h.scenicSpotServiceError(c, err)
		return
	}
	primary := &models.TrackPrimaryScenicSpot{ID: spot.ID, Name: spot.Name, ScenicGrade: spot.ScenicGrade, Category: spot.Category, Description: spot.Description, Address: spot.Address, Sources: spot.Sources}
	c.JSON(http.StatusOK, utils.H{"code": 0, "data": utils.H{
		"primary_scenic_spot": primary,
		"scenic_spot_id":      spot.ID,
		"labels":              service.TrackDisplayLabels(&models.Track{PrimaryScenicSpot: primary}),
	}})
}
