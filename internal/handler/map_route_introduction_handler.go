package handler

import (
	"bytes"
	"context"
	_ "embed"
	"errors"
	"fmt"
	"html/template"
	"net/http"
	"strconv"
	"strings"

	"github.com/cloudwego/hertz/pkg/app"
	"github.com/tongyichu/track_server/internal/repository"
	"github.com/tongyichu/track_server/internal/service"
)

//go:embed static/map_route_introduction.html
var mapRouteIntroductionHTML string

var mapRouteIntroductionTemplate = template.Must(template.New("map-route-introduction").Parse(mapRouteIntroductionHTML))

type MapRouteIntroductionHandler struct{ service *service.TrackMapService }

type mapRouteIntroductionPageData struct {
	Language, Name, Summary, TrackType, Difficulty           string
	Dark                                                     bool
	Description, Highlights, Tips, BestSeasons               []string
	Distance                                                 string
	Duration                                                 string
	HighlightsTitle, TipsTitle, DescriptionTitle, FactsTitle string
}

func NewMapRouteIntroductionHandler(service *service.TrackMapService) *MapRouteIntroductionHandler {
	return &MapRouteIntroductionHandler{service: service}
}

func (h *MapRouteIntroductionHandler) IntroductionPage(ctx context.Context, c *app.RequestContext) {
	if h == nil || h.service == nil {
		c.String(http.StatusNotFound, "route introduction not found")
		return
	}
	group, introduction, err := h.service.GetPublishedRouteIntroduction(ctx, c.Param("group_id"))
	if err != nil {
		if errors.Is(err, repository.ErrNotFound) {
			c.String(http.StatusNotFound, "route introduction not found")
			return
		}
		c.String(http.StatusInternalServerError, "failed to load route introduction")
		return
	}
	english := strings.EqualFold(strings.TrimSpace(string(c.Query("lang"))), "english")
	content := introduction.Chinese
	data := mapRouteIntroductionPageData{Language: "zh-CN", Dark: strings.EqualFold(strings.TrimSpace(string(c.Query("is_dark"))), "true"),
		TrackType: string(group.TrackType), Difficulty: introduction.Difficulty, BestSeasons: introduction.BestSeasons,
		Distance: formatRouteDistance(group.Distance), Duration: formatRouteDuration(introduction.EstimatedDurationMin, introduction.EstimatedDurationMax),
		DescriptionTitle: "路线介绍", HighlightsTitle: "路线亮点", TipsTitle: "出行提示", FactsTitle: "路线信息"}
	if english && strings.TrimSpace(introduction.English.Name) != "" {
		content = introduction.English
		data.Language, data.DescriptionTitle, data.HighlightsTitle, data.TipsTitle, data.FactsTitle = "en", "About the Route", "Highlights", "Before You Go", "Route Facts"
	}
	data.Name, data.Summary, data.Description, data.Highlights, data.Tips = content.Name, content.Summary, content.Description, content.Highlights, content.Tips
	var rendered bytes.Buffer
	if err := mapRouteIntroductionTemplate.Execute(&rendered, data); err != nil {
		c.String(http.StatusInternalServerError, "failed to render route introduction")
		return
	}
	c.Response.Header.Set("Cache-Control", "public, max-age=300")
	c.Response.Header.Set("Content-Security-Policy", "default-src 'none'; style-src 'unsafe-inline'; img-src https: data:; base-uri 'none'; frame-ancestors *")
	c.Response.Header.Set("X-Content-Type-Options", "nosniff")
	c.Data(http.StatusOK, "text/html; charset=utf-8", rendered.Bytes())
}

func formatRouteDistance(distance float64) string {
	if distance <= 0 {
		return ""
	}
	return strings.TrimRight(strings.TrimRight(strconv.FormatFloat(distance/1000, 'f', 1, 64), "0"), ".") + " km"
}

func formatRouteDuration(min, max int) string {
	if min <= 0 && max <= 0 {
		return ""
	}
	if max <= 0 || max == min {
		return formatMinutes(min)
	}
	return formatMinutes(min) + " – " + formatMinutes(max)
}

func formatMinutes(minutes int) string {
	if minutes < 60 {
		return fmt.Sprintf("%d min", minutes)
	}
	if minutes%60 == 0 {
		return fmt.Sprintf("%d h", minutes/60)
	}
	return fmt.Sprintf("%d h %d min", minutes/60, minutes%60)
}
