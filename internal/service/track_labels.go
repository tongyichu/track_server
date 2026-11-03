package service

import (
	"strings"

	"github.com/tongyichu/track_server/internal/config"
	"github.com/tongyichu/track_server/internal/models"
)

func buildTrackLabels(shape, sport string, spot *models.TrackPrimaryScenicSpot) []models.TrackLabel {
	labels := make([]models.TrackLabel, 0, 3)
	if shape = strings.TrimSpace(shape); shape != "" {
		labels = append(labels, models.TrackLabel{Type: "shape", Value: shape, Name: shape, ThemeColor: config.TrackShapeLabelColor, ThemeColorDark: config.TrackShapeLabelColorDark})
	}
	if spot != nil && strings.TrimSpace(spot.ID) != "" && strings.TrimSpace(spot.Name) != "" {
		labels = append(labels, models.TrackLabel{Type: "scenic_spot", Value: spot.ID, Name: strings.TrimSpace(spot.Name), ThemeColor: config.TrackScenicLabelColor, ThemeColorDark: config.TrackScenicLabelColorDark})
	}
	if sport = strings.TrimSpace(sport); sport != "" {
		code := normalizeTrackTypeCode(sport)
		label := models.TrackLabel{Type: "sport", Value: code, Name: sport}
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
	return labels
}
