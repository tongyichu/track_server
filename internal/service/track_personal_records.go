package service

import (
	"context"
	"errors"
	"sort"
	"strings"

	"github.com/tongyichu/track_server/internal/models"
	"github.com/tongyichu/track_server/internal/repository"
)

func (s *TrackService) PersonalRecords(ctx context.Context, userID int64, trackType string) (*models.PersonalRecords, error) {
	if userID <= 0 {
		return nil, invalidArg("userID is required")
	}
	selected := normalizeTrackTypeCode(strings.TrimSpace(trackType))
	if len(selected) > 32 {
		return nil, invalidArg("invalid track_type")
	}
	repo, ok := s.tracks.(repository.PersonalRecordsRepository)
	if !ok {
		return nil, errors.New("personal records unavailable")
	}
	ids, err := repo.FindPersonalRecordIDs(ctx, userID)
	if err != nil {
		return nil, err
	}
	tracks, err := s.tracks.FindByIDs(ctx, ids)
	if err != nil {
		return nil, err
	}
	winners := make(map[string]map[string]*models.Track)
	for _, t := range tracks {
		if t == nil || t.UserID != userID || t.IsRunning || strings.TrimSpace(t.RawTrackURL) == "" ||
			(t.Status != models.TrackStatusNormal && t.Status != models.TrackStatusPrivate) {
			continue
		}
		code := normalizeTrackTypeCode(t.TrackType)
		if code == "" {
			code = "other"
		}
		if selected != "" && selected != code {
			continue
		}
		if winners[code] == nil {
			winners[code] = make(map[string]*models.Track)
		}
		for _, metric := range []string{"distance", "elevation", "duration", "steps"} {
			if (metric == "steps" && (code == "driving" || code == "riding")) || (metric == "elevation" && code == "driving") {
				continue
			}
			value := models.PersonalRecordValue(t, metric)
			if value <= 0 {
				continue
			}
			old := winners[code][metric]
			if old == nil || value > models.PersonalRecordValue(old, metric) ||
				(value == models.PersonalRecordValue(old, metric) && (t.StartTime.After(old.StartTime) ||
					(t.StartTime.Equal(old.StartTime) && t.ID > old.ID))) {
				winners[code][metric] = t
			}
		}
	}
	result := &models.PersonalRecords{Groups: make([]models.PersonalRecordGroup, 0, len(winners))}
	for code, records := range winners {
		group := models.PersonalRecordGroup{TrackType: code, Records: make([]models.PersonalRecord, 0, len(records))}
		for _, metric := range []string{"distance", "elevation", "duration", "steps"} {
			if t := records[metric]; t != nil {
				// Reuse authenticated preview routes without fetching images or GPS files.
				summary := toSummaries([]*models.Track{t})[0]
				group.Records = append(group.Records, models.PersonalRecord{Metric: metric, Value: models.PersonalRecordValue(t, metric), Track: summary})
			}
		}
		if len(group.Records) > 0 {
			result.Groups = append(result.Groups, group)
		}
	}
	sort.Slice(result.Groups, func(i, j int) bool { return result.Groups[i].TrackType < result.Groups[j].TrackType })
	return result, nil
}
