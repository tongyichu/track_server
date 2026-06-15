package handler

import (
	"context"
	"errors"
	"net/http"
	"strconv"

	"github.com/cloudwego/hertz/pkg/app"
	"github.com/cloudwego/hertz/pkg/common/utils"

	"github.com/tongyichu/track_server/internal/middleware"
	"github.com/tongyichu/track_server/internal/repository"
	"github.com/tongyichu/track_server/internal/service"
)

// TrackMapHandler handles homepage map mode APIs.
type TrackMapHandler struct {
	trackMapSvc *service.TrackMapService
}

func NewTrackMapHandler(trackMapSvc *service.TrackMapService) *TrackMapHandler {
	return &TrackMapHandler{trackMapSvc: trackMapSvc}
}

// View handles GET /api/v1/track-map/view.
func (h *TrackMapHandler) View(ctx context.Context, c *app.RequestContext) {
	if !ensureAuthenticated(c) {
		return
	}
	input, err := parseTrackMapViewInput(c)
	if err != nil {
		c.JSON(http.StatusBadRequest, utils.H{"error": err.Error()})
		return
	}
	resp, err := h.trackMapSvc.View(ctx, input)
	if err != nil {
		writeTrackMapError(c, err)
		return
	}
	c.JSON(http.StatusOK, successResponse(resp))
}

// ListGroups handles GET /api/v1/track-map/groups.
func (h *TrackMapHandler) ListGroups(ctx context.Context, c *app.RequestContext) {
	if !ensureAuthenticated(c) {
		return
	}
	input, err := parseTrackMapViewInput(c)
	if err != nil {
		c.JSON(http.StatusBadRequest, utils.H{"error": err.Error()})
		return
	}
	resp, err := h.trackMapSvc.ListGroups(ctx, input)
	if err != nil {
		writeTrackMapError(c, err)
		return
	}
	c.JSON(http.StatusOK, successResponse(resp))
}

// GetGroupDetail handles GET /api/v1/track-map/groups/:group_id/detail.
func (h *TrackMapHandler) GetGroupDetail(ctx context.Context, c *app.RequestContext) {
	if !ensureAuthenticated(c) {
		return
	}
	resp, err := h.trackMapSvc.GetGroupDetail(ctx, c.Param("group_id"))
	if err != nil {
		writeTrackMapError(c, err)
		return
	}
	c.JSON(http.StatusOK, successResponse(resp))
}

// ListGroupTracks handles GET /api/v1/track-map/groups/:group_id/tracks.
func (h *TrackMapHandler) ListGroupTracks(ctx context.Context, c *app.RequestContext) {
	meta := middleware.GetRequestMeta(c)
	if meta == nil || meta.AuthUserID <= 0 {
		c.JSON(http.StatusUnauthorized, utils.H{"error": "unauthorized"})
		return
	}
	limit := parseQueryInt(c, "limit")
	resp, err := h.trackMapSvc.ListGroupTracks(ctx, meta.AuthUserID, c.Param("group_id"), service.TrackMapGroupTracksInput{Limit: limit})
	if err != nil {
		writeTrackMapError(c, err)
		return
	}
	c.JSON(http.StatusOK, successResponse(resp))
}

func ensureAuthenticated(c *app.RequestContext) bool {
	meta := middleware.GetRequestMeta(c)
	if meta == nil || meta.AuthUserID <= 0 {
		c.JSON(http.StatusUnauthorized, utils.H{"error": "unauthorized"})
		return false
	}
	return true
}

func parseTrackMapViewInput(c *app.RequestContext) (service.TrackMapViewInput, error) {
	input := service.TrackMapViewInput{
		BBox:      string(c.Query("bbox")),
		CityCode:  string(c.Query("city_code")),
		TrackType: string(c.Query("track_type")),
		RadiusM:   parseQueryInt(c, "radius_m"),
		Limit:     parseQueryInt(c, "limit"),
	}
	if zoomRaw := string(c.Query("zoom")); zoomRaw != "" {
		zoom, err := strconv.ParseFloat(zoomRaw, 64)
		if err != nil {
			return input, &service.InvalidArgumentError{Msg: "invalid zoom"}
		}
		input.Zoom = zoom
	}
	if latRaw := string(c.Query("latitude")); latRaw != "" {
		lat, err := strconv.ParseFloat(latRaw, 64)
		if err != nil {
			return input, &service.InvalidArgumentError{Msg: "invalid latitude"}
		}
		input.Latitude = &lat
	}
	if lonRaw := string(c.Query("longitude")); lonRaw != "" {
		lon, err := strconv.ParseFloat(lonRaw, 64)
		if err != nil {
			return input, &service.InvalidArgumentError{Msg: "invalid longitude"}
		}
		input.Longitude = &lon
	}
	return input, nil
}

func parseQueryInt(c *app.RequestContext, key string) int {
	raw := string(c.Query(key))
	if raw == "" {
		return 0
	}
	v, err := strconv.Atoi(raw)
	if err != nil {
		return 0
	}
	return v
}

func writeTrackMapError(c *app.RequestContext, err error) {
	switch {
	case errors.Is(err, repository.ErrNotFound):
		c.JSON(http.StatusNotFound, utils.H{"error": err.Error()})
	case errors.As(err, new(*service.InvalidArgumentError)):
		c.JSON(http.StatusBadRequest, utils.H{"error": err.Error()})
	default:
		c.JSON(http.StatusInternalServerError, utils.H{"error": err.Error()})
	}
}
