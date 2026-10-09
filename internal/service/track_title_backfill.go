package service

import (
	"context"
	"errors"
	"fmt"
	"math"
	"regexp"
	"strings"
	"unicode/utf8"

	"github.com/tongyichu/track_server/internal/config"
	"github.com/tongyichu/track_server/internal/models"
	"github.com/tongyichu/track_server/internal/repository"
)

// TrackTitleChange is a reviewable before/after record for the one-off backfill.
type TrackTitleChange struct {
	TrackID       string `json:"track_id"`
	UserID        int64  `json:"user_id"`
	OldTitle      string `json:"old_title"`
	NewTitle      string `json:"new_title"`
	Source        string `json:"source"`
	GeometryKnown bool   `json:"geometry_known"`
}

var numericDateTitle = regexp.MustCompile(`^(?:[0-9]{4}[-/.年][0-9]{1,2}[-/.月][0-9]{1,2}日?|[0-9]{1,2}[-/.][0-9]{1,2}[-/.][0-9]{4})(?:[T\s,•·]+(?:上午|下午)?[0-9]{1,2}[:时][0-9]{2}(?:[:分][0-9]{2}(?:\.[0-9]+)?)?秒?(?:\s*(?:AM|PM|am|pm|Z|GMT|UTC|[+-][0-9]{2}:?[0-9]{2}))*)?$`)
var leadingTitleDate = regexp.MustCompile(`^[0-9]{4}[-/.年][0-9]{1,2}[-/.月][0-9]{1,2}日?(?:[T\s,•·]+(?:上午|下午)?[0-9]{1,2}[:时][0-9]{2}(?:[:分][0-9]{2}(?:\.[0-9]+)?)?秒?(?:\s*(?:AM|PM|am|pm|Z|GMT|UTC|[+-][0-9]{2}:?[0-9]{2}))*)?`)
var trailingTitleDate = regexp.MustCompile(`\s+[0-9]{4}[-/.年][0-9]{1,2}[-/.月][0-9]{1,2}日?$`)
var fallbackTitleDescription = regexp.MustCompile(`^(?:徒步|跑步|爬山|骑行|自驾)(?:路线)?记录$`)
var englishDateTitle = regexp.MustCompile(`(?i)^(?:(?:Jan|Feb|Mar|Apr|May|Jun|Jul|Aug|Sep|Oct|Nov|Dec)[a-z]*\s+[0-9]{1,2},?\s+[0-9]{4}|[0-9]{1,2}\s+(?:Jan|Feb|Mar|Apr|May|Jun|Jul|Aug|Sep|Oct|Nov|Dec)[a-z]*\s+[0-9]{4})(?:,?\s+[0-9]{1,2}:[0-9]{2}(?::[0-9]{2})?(?:\s*(?:AM|PM))?)?$`)
var administrativeLocation = regexp.MustCompile(`(?:省|市|区|县|自治区|自治州|地区)$`)
var addressNumberOnly = regexp.MustCompile(`(?i)^(?:(?:甲|乙|丙|丁)?[0-9]+(?:[-－][0-9]+)*(?:号(?:院|楼)?|栋|幢|单元)?|No\.?\s*[0-9]+)$`)
var locationPlusCode = regexp.MustCompile(`(?i)^[23456789CFGHJMPQRVWX]{2,8}\+[23456789CFGHJMPQRVWX]{2,3}$`)
var coordinateLocation = regexp.MustCompile(`^[+-]?[0-9]+(?:\.[0-9]+)?\s*[,，]\s*[+-]?[0-9]+(?:\.[0-9]+)?$`)

// IsDateOnlyTrackTitle excludes descriptive titles even when they contain a date.
func IsDateOnlyTrackTitle(title string) bool {
	title = strings.TrimSpace(title)
	return numericDateTitle.MatchString(title) || englishDateTitle.MatchString(title)
}

// IsDatedTrackTitle is used only for the explicitly requested expanded cleanup.
// Dates inside a descriptive title are not treated as a timestamp prefix/suffix.
func IsDatedTrackTitle(title string) bool {
	title = strings.TrimSpace(title)
	return IsDateOnlyTrackTitle(title) || leadingTitleDate.MatchString(title) || trailingTitleDate.MatchString(title)
}

func descriptionWithoutTitleDate(title string) string {
	title = strings.TrimSpace(title)
	if IsDateOnlyTrackTitle(title) {
		return ""
	}
	title = leadingTitleDate.ReplaceAllString(title, "")
	title = trailingTitleDate.ReplaceAllString(title, "")
	title = strings.Trim(title, " \t\n•·,，-—")
	if fallbackTitleDescription.MatchString(title) {
		return ""
	}
	return title
}

// Validate requires a timestamp on the old title and a descriptive replacement.
func (c TrackTitleChange) Validate() error {
	if strings.TrimSpace(c.TrackID) == "" || c.UserID <= 0 || !IsDatedTrackTitle(c.OldTitle) {
		return errors.New("invalid track identity or non-date original title")
	}
	if c.NewTitle != strings.TrimSpace(c.NewTitle) || utf8.RuneCountInString(c.NewTitle) < 4 ||
		utf8.RuneCountInString(c.NewTitle) > 40 || IsDatedTrackTitle(c.NewTitle) {
		return errors.New("replacement title must be descriptive and 4-40 characters")
	}
	return nil
}

// TrackTitleBackfillPlanner reuses recorded facts and the existing raw-track parser.
// It never writes to the database or calls reverse-geocoding services.
type TrackTitleBackfillPlanner struct {
	Submissions        repository.TrackSubmissionRepository
	RawTracks          *AssetCacheService
	IncludeDatedTitles bool
}

func (p TrackTitleBackfillPlanner) Plan(ctx context.Context, track *models.Track) (*TrackTitleChange, error) {
	if track == nil || track.IsRunning || track.Status == models.TrackStatusDeleted {
		return nil, nil
	}
	if !IsDateOnlyTrackTitle(track.Title) && !(p.IncludeDatedTitles && IsDatedTrackTitle(track.Title)) {
		return nil, nil
	}
	change := &TrackTitleChange{TrackID: track.ID, UserID: track.UserID, OldTitle: track.Title}
	if p.Submissions != nil {
		sub, err := p.Submissions.FindByTrackID(ctx, track.ID)
		if err != nil && !errors.Is(err, repository.ErrNotFound) {
			return nil, err
		}
		if sub != nil && sub.TrackID == track.ID && sub.UserID == track.UserID {
			change.NewTitle = strings.TrimSpace(sub.Title)
			if change.Validate() == nil {
				change.Source = "submission"
				return change, nil
			}
			change.NewTitle = ""
		}
	}
	points := track.Points
	if len(points) < 2 && p.RawTracks != nil && track.RawTrackURL != "" {
		path, err := p.RawTracks.EnsureCachedFile(ctx, track.UserID, track.ID, track.RawTrackURL)
		if err == nil {
			points, _ = parseTrackPointsFile(path)
		}
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	change.GeometryKnown = len(validTitlePoints(points)) >= 2
	change.Source = "recorded_facts"
	if p.IncludeDatedTitles {
		description := descriptionWithoutTitleDate(track.Title)
		if description != "" {
			if administrativeLocation.MatchString(description) {
				// Imported date + district titles contain an actual recorded location.
				if specificTitleLocation(track.LocateAddr) == "" {
					change.NewTitle = description + "周边" + trackTitleSport(track.TrackType)
				}
			} else {
				change.NewTitle = description
				if utf8.RuneCountInString(description) < 4 {
					change.NewTitle += trackTitleSport(track.TrackType) + "路线"
				}
				change.Source = "date_removed"
			}
		}
	}
	if change.NewTitle == "" {
		change.NewTitle = generatedTrackTitle(track, points)
	}
	if change.NewTitle == "" {
		return nil, nil
	}
	if err := change.Validate(); err != nil {
		return nil, err
	}
	return change, nil
}

func trackTitleSport(trackType string) string {
	for _, item := range config.DefaultTrackTypeConfigs {
		if item.Type == normalizeTrackTypeCode(trackType) {
			return item.Name
		}
	}
	return ""
}

func generatedTrackTitle(track *models.Track, points []models.TrackPoint) string {
	sport := trackTitleSport(track.TrackType)
	if sport == "" {
		return ""
	}
	fallback := sport + "路线记录"
	if !math.IsNaN(track.Distance) && !math.IsInf(track.Distance, 0) && track.Distance >= 1 {
		if track.Distance < 1000 {
			fallback = fmt.Sprintf("%.0f米%s路线", track.Distance, sport)
		} else {
			fallback = fmt.Sprintf("%.1f公里%s路线", track.Distance/1000, sport)
		}
	}
	start := specificTitleLocation(track.LocateAddr)
	if start == "" {
		return fallback
	}
	nearby := start + "周边" + sport
	if utf8.RuneCountInString(nearby) > 40 {
		return fallback
	}
	loop := nearby + "环线"
	if isTitleLoop(points, track.Distance) && utf8.RuneCountInString(loop) <= 40 {
		return loop
	}
	// Only one locate_addr is persisted remotely; do not invent an endpoint address.
	return nearby
}

func specificTitleLocation(label string) string {
	parts := strings.Split(label, "·")
	for i := len(parts) - 1; i >= 0; i-- {
		part := strings.TrimSpace(parts[i])
		// Older locate_addr values were truncated; keep the intact place before an open bracket.
		for _, pair := range [][2]string{{"(", ")"}, {"（", "）"}} {
			if index := strings.Index(part, pair[0]); index >= 0 && !strings.Contains(part[index:], pair[1]) {
				part = strings.TrimSpace(part[:index])
			}
		}
		switch strings.ToLower(part) {
		case "已定位", "正在定位", "定位中", "未定位", "位置未知", "未知位置", "located", "locating", "unknown location":
			continue
		}
		if part != "" && utf8.RuneCountInString(part) <= 40 && !strings.EqualFold(part, "null") &&
			!strings.EqualFold(part, "unknown") && !administrativeLocation.MatchString(part) &&
			!addressNumberOnly.MatchString(part) && !locationPlusCode.MatchString(part) && !coordinateLocation.MatchString(part) {
			return part
		}
	}
	return ""
}

func validTitlePoints(points []models.TrackPoint) []models.TrackPoint {
	valid := make([]models.TrackPoint, 0, len(points))
	for _, point := range points {
		if !math.IsNaN(point.Latitude) && !math.IsNaN(point.Longitude) &&
			point.Latitude >= -90 && point.Latitude <= 90 && point.Longitude >= -180 && point.Longitude <= 180 {
			valid = append(valid, point)
		}
	}
	return valid
}

func isTitleLoop(points []models.TrackPoint, distance float64) bool {
	points = validTitlePoints(points)
	if len(points) < 4 || math.IsNaN(distance) || math.IsInf(distance, 0) || distance < 500 {
		return false
	}
	first, last := points[0], points[len(points)-1]
	if haversineDistance(first.Latitude, first.Longitude, last.Latitude, last.Longitude) > math.Min(150, distance*0.05) {
		return false
	}
	const metersPerDegree = 111195.0
	longitudeScale := metersPerDegree * math.Cos(first.Latitude*math.Pi/180)
	twiceArea := 0.0
	for i := 1; i < len(points); i++ {
		a, b := points[i-1], points[i]
		ax, ay := (a.Longitude-first.Longitude)*longitudeScale, (a.Latitude-first.Latitude)*metersPerDegree
		bx, by := (b.Longitude-first.Longitude)*longitudeScale, (b.Latitude-first.Latitude)*metersPerDegree
		twiceArea += ax*by - bx*ay
	}
	return math.Abs(twiceArea)/2 >= distance*distance*0.01
}
