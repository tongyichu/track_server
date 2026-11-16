package service

import (
	"strings"

	"github.com/tongyichu/track_server/internal/config"
	"github.com/tongyichu/track_server/internal/models"
)

func buildTrackLabels(shape, sport string, spot *models.TrackPrimaryScenicSpot, featured ...bool) []models.TrackLabel {
	labels := make([]models.TrackLabel, 0, 4)
	if len(featured) > 0 && featured[0] {
		labels = append(labels, models.TrackLabel{Type: "featured", Value: true, Code: "featured", Name: "精品", NameEN: "Featured", ThemeColor: config.TrackFeaturedLabelColor, ThemeColorDark: config.TrackFeaturedLabelColorDark})
	}
	if sport = strings.TrimSpace(sport); sport != "" {
		code := normalizeTrackTypeCode(sport)
		label := models.TrackLabel{Type: "sport", Value: true, Code: code, Name: sport}
		for _, item := range config.DefaultTrackTypeConfigs {
			if item.Type == code {
				label.Name = item.Name
				label.NameEN = item.NameEN
				label.ThemeColor = item.ThemeColor
				label.ThemeColorDark = item.ThemeColorDark
				break
			}
		}
		labels = append(labels, label)
	}
	if spot != nil && strings.TrimSpace(spot.ID) != "" && strings.TrimSpace(spot.Name) != "" {
		name := strings.TrimSpace(spot.Name)
		if grade := normalizeScenicGrade(spot.ScenicGrade); grade != "" {
			name += "·" + grade
		}
		labels = append(labels, models.TrackLabel{Type: "scenic_spot", Value: true, Code: spot.ID, Name: name, ThemeColor: config.TrackScenicLabelColor, ThemeColorDark: config.TrackScenicLabelColorDark})
	}
	if shape = strings.TrimSpace(shape); shape != "" {
		labels = append(labels, models.TrackLabel{Type: "shape", Value: true, Code: shape, Name: shape, ThemeColor: config.TrackShapeLabelColor, ThemeColorDark: config.TrackShapeLabelColorDark})
	}
	return labels
}

func TrackDisplayLabels(track *models.Track) []models.TrackLabel {
	return buildTrackDisplayLabels(track.ShapeLabel, track.TrackType, track.PrimaryScenicSpot, track.Submission != nil && track.Submission.Status == models.TrackSubmissionStatusApproved, track.IsCompanion)
}

func buildTrackDisplayLabels(shape, sport string, spot *models.TrackPrimaryScenicSpot, featured, companion bool) []models.TrackLabel {
	base := buildTrackLabels(shape, sport, spot, featured)
	if !companion {
		return base
	}
	index := 0
	for index < len(base) && (base[index].Type == "featured" || base[index].Type == "sport") {
		index++
	}
	labels := make([]models.TrackLabel, 0, len(base)+1)
	labels = append(labels, base[:index]...)
	labels = append(labels, models.TrackLabel{Type: "companion", Value: true, Code: "companion", Name: "与友同行", NameEN: "With Friends", ThemeColor: config.TrackCompanionLabelColor, ThemeColorDark: config.TrackCompanionLabelColorDark})
	return append(labels, base[index:]...)
}
