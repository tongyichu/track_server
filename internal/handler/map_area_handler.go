package handler

import (
	"bytes"
	"context"
	_ "embed"
	"html/template"
	"net/http"
	"strings"

	"github.com/cloudwego/hertz/pkg/app"
	"github.com/tongyichu/track_server/internal/config"
	"github.com/tongyichu/track_server/internal/maparea"
)

//go:embed static/map_area_introduction.html
var mapAreaIntroductionHTML string

var mapAreaIntroductionTemplate = template.Must(template.New("map-area-introduction").Parse(mapAreaIntroductionHTML))

type MapAreaHandler struct {
	catalog *maparea.Catalog
}

type mapAreaPageData struct {
	Language        string
	Dark            bool
	Name            string
	CityName        string
	Summary         string
	Highlights      []string
	Tips            []string
	HighlightsTitle string
	TipsTitle       string
	AreaTypeLabel   string
}

func NewMapAreaHandler(catalog *maparea.Catalog) *MapAreaHandler {
	return &MapAreaHandler{catalog: catalog}
}

// IntroductionPage serves the public map-area introduction H5 page.
func (h *MapAreaHandler) IntroductionPage(_ context.Context, c *app.RequestContext) {
	area := h.catalog.Find(c.Param("area_id"))
	if area == nil || !area.HasIntroduction() {
		c.String(http.StatusNotFound, "map area introduction not found")
		return
	}

	english := strings.EqualFold(strings.TrimSpace(string(c.Query("lang"))), "english")
	content := area.Chinese
	data := mapAreaPageData{
		Language:        "zh-CN",
		Dark:            strings.EqualFold(strings.TrimSpace(string(c.Query("is_dark"))), "true"),
		Name:            content.Name,
		CityName:        config.CityNameByCode(area.CityCode),
		Summary:         content.Summary,
		Highlights:      content.Highlights,
		Tips:            content.Tips,
		HighlightsTitle: "区域特色",
		TipsTitle:       "出行提示",
		AreaTypeLabel:   mapAreaTypeLabel(area.Type, false),
	}
	if english {
		content = area.English
		if strings.TrimSpace(content.Name) == "" {
			content = area.Chinese
		}
		data.Language = "en"
		data.Name = content.Name
		data.CityName = area.CityNameEnglish
		data.Summary = content.Summary
		data.Highlights = content.Highlights
		data.Tips = content.Tips
		data.HighlightsTitle = "Highlights"
		data.TipsTitle = "Before You Go"
		data.AreaTypeLabel = mapAreaTypeLabel(area.Type, true)
	}

	var rendered bytes.Buffer
	if err := mapAreaIntroductionTemplate.Execute(&rendered, data); err != nil {
		c.String(http.StatusInternalServerError, "failed to render map area introduction")
		return
	}
	c.Response.Header.Set("Cache-Control", "public, max-age=300")
	c.Response.Header.Set("Content-Security-Policy", "default-src 'none'; style-src 'unsafe-inline'; img-src https: data:; base-uri 'none'; frame-ancestors *")
	c.Response.Header.Set("X-Content-Type-Options", "nosniff")
	c.Data(http.StatusOK, "text/html; charset=utf-8", rendered.Bytes())
}

func mapAreaTypeLabel(areaType string, english bool) string {
	if english {
		if areaType == maparea.TypeScenicSpot {
			return "Scenic area"
		}
		return "District"
	}
	if areaType == maparea.TypeScenicSpot {
		return "景区"
	}
	return "区县"
}
