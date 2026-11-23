package service

import (
	"context"
	"fmt"
	"math"
	"slices"
	"strings"

	"github.com/tongyichu/track_server/internal/config"
	"github.com/tongyichu/track_server/internal/maparea"
	"github.com/tongyichu/track_server/internal/models"
	"github.com/tongyichu/track_server/internal/repository"
)

const routeOverviewSampleLimit = 50

type routeGroupFacts struct {
	Start       string
	Finish      string
	Scenic      string
	Form        string
	TrackType   string
	Distance    routeMetric
	Duration    routeMetric
	Elevation   routeMetric
	SampleCount int
}

type routeMetric struct{ Low, High, Count int }

func (s *TrackMapService) loadRouteGroupFacts(ctx context.Context, group *models.TrackRouteGroup) (routeGroupFacts, error) {
	members, err := s.maps.ListRouteGroupMembers(ctx, group.GroupID, routeOverviewSampleLimit)
	if err != nil {
		return routeGroupFacts{}, err
	}
	ids := make([]string, 0, len(members))
	for _, member := range members {
		if member != nil {
			ids = append(ids, member.TrackID)
		}
	}
	if len(ids) == 0 || s.tracks == nil {
		return routeGroupFacts{}, repository.ErrNotFound
	}
	tracks, err := s.tracks.FindByIDs(ctx, ids)
	if err != nil {
		return routeGroupFacts{}, err
	}
	ids = slices.DeleteFunc(ids, func(id string) bool {
		track := tracks[id]
		if !publicMapTrack(track) {
			return true
		}
		trackType := normalizeTrackTypeCode(track.TrackType)
		return trackType != "" && trackType != normalizeTrackTypeCode(group.TrackType)
	})
	if len(ids) == 0 {
		return routeGroupFacts{}, repository.ErrNotFound
	}
	reader, ok := s.maps.(repository.TrackPlaceIndexReader)
	if !ok {
		return routeGroupFacts{}, repository.ErrNotFound
	}
	indexes, err := reader.FindPlaceGeoIndexes(ctx, ids)
	if err != nil {
		return routeGroupFacts{}, err
	}
	// The shared matcher sorts its input; keep the member order for representative samples.
	matches, err := s.trackScenicMatches(ctx, slices.Clone(ids))
	if err != nil {
		return routeGroupFacts{}, err
	}
	refID := group.RepresentativeTrackID
	if !slices.Contains(ids, refID) {
		refID = ids[0]
	}
	ref := indexes[refID]
	if ref == nil {
		for _, id := range ids {
			if indexes[id] != nil {
				refID, ref = id, indexes[id]
				break
			}
		}
	}
	facts := routeGroupLocationFacts(s.areas, group, ref, tracks[refID], routeGroupScenic(group, ids, matches))
	seenSessions := make(map[string]bool)
	type routeEndpoints struct{ start, finish string }
	endpoints := make(map[routeEndpoints]int)
	forms := make(map[string]int)
	var distances, durations, elevations []int
	orderedIDs := append([]string{refID}, ids...)
	for _, id := range orderedIDs {
		track := tracks[id]
		session := "track:" + id
		if track.SessionID != "" {
			session = "session:" + track.SessionID
		}
		if seenSessions[session] {
			continue
		}
		seenSessions[session] = true
		facts.SampleCount++
		index := indexes[id]
		location := routeGroupLocationFacts(s.areas, group, index, track, "")
		if location.Form != "" {
			forms[location.Form]++
		}
		if location.Start != "" {
			endpoints[routeEndpoints{location.Start, location.Finish}]++
		}
		distance := track.Distance
		if index != nil && !validRouteMetricDistance(distance) {
			distance = index.Distance
		}
		if validRouteMetricDistance(distance) {
			distances = append(distances, int(math.Round(distance)))
		}
		if track.Duration > 0 {
			durations = append(durations, max(1, int(math.Round(float64(track.Duration)/60))))
		}
		if track.ElevationGain > 0 {
			elevations = append(elevations, track.ElevationGain)
		}
	}
	if facts.SampleCount > 1 {
		facts.Start, facts.Finish, facts.Form = "", "", ""
		for form, count := range forms {
			if count*2 > facts.SampleCount {
				facts.Form = form
			}
		}
		for pair, count := range endpoints {
			if count*2 > facts.SampleCount {
				facts.Start, facts.Finish = pair.start, pair.finish
			}
		}
		if facts.Form != "point_to_point" {
			facts.Finish = ""
		}
	}
	facts.Distance, facts.Duration, facts.Elevation = routeMetricFrom(distances), routeMetricFrom(durations), routeMetricFrom(elevations)
	return facts, nil
}

func validRouteMetricDistance(distance float64) bool {
	return distance >= minRouteGroupDistanceMeters && distance < float64(math.MaxInt32)
}

func routeMetricFrom(values []int) routeMetric {
	switch len(values) {
	case 0:
		return routeMetric{}
	case 1, 2:
		return routeMetric{Low: values[0], High: values[0], Count: 1}
	default:
		slices.Sort(values)
		last := len(values) - 1
		return routeMetric{Low: values[last/4], High: values[(3*last+2)/4], Count: len(values)}
	}
}

func routeGroupScenic(group *models.TrackRouteGroup, ids []string, matches map[string]*departureScenicMatch) string {
	var chosen mapDeparture
	for _, id := range ids {
		if match := matches[id]; match != nil {
			d := mapDeparture{TrackMapDeparture: models.TrackMapDeparture{
				GroupID: group.GroupID, TrackID: id, RepresentativeTrackID: group.RepresentativeTrackID,
			}, scenic: match}
			if chosen.scenic == nil || preferScenicDeparture(d, chosen) {
				chosen = d
			}
		}
	}
	if chosen.scenic != nil {
		return chosen.scenic.name
	}
	return ""
}

func routeGroupLocationFacts(areas *maparea.Catalog, group *models.TrackRouteGroup, index *models.TrackGeoIndex, track *models.Track, scenic string) routeGroupFacts {
	facts := routeGroupFacts{Scenic: scenic, TrackType: trackTypeDisplayName(group.TrackType)}
	if len(group.CityCodes) == 1 {
		facts.Start = config.CityNameByCode(config.CanonicalCityCode(group.CityCodes[0]))
	}
	if index == nil {
		return facts
	}
	if start := areas.ResolveDistrict(index.StartLat, index.StartLng); start != nil {
		facts.Start = start.Name()
	} else if city := config.CityNameByCode(config.CanonicalCityCode(index.CityCode)); city != "" {
		facts.Start = city
	}
	if end := areas.ResolveDistrict(index.EndLat, index.EndLng); end != nil && end.Name() != facts.Start {
		facts.Finish = end.Name()
	}
	if track != nil {
		if place := routeStartLandmark(track.LocateAddr); place != "" {
			facts.Start = place
		}
	}
	facts.Form = routeGroupForm(index)
	if facts.Form != "point_to_point" {
		facts.Finish = ""
	}
	return facts
}

func routeStartLandmark(address string) string {
	place := specificTitleLocation(address)
	for _, suffix := range []string{"公园", "广场", "绿道", "步道", "栈道", "古道", "入口", "登山口", "山门", "车站", "码头", "大桥", "水库"} {
		if strings.HasSuffix(place, suffix) {
			return place
		}
	}
	return ""
}

func routeGroupForm(index *models.TrackGeoIndex) string {
	if isTitleLoop(index.SimplifiedPolyline, index.Distance) {
		return "loop"
	}
	points := resampleRoutePolyline(index)
	if len(points) < 4 || index.Distance < 500 {
		return ""
	}
	first, last := points[0], points[len(points)-1]
	closeEnough := math.Min(150, index.Distance*0.05)
	if haversineMeters(first.Latitude, first.Longitude, last.Latitude, last.Longitude) > closeEnough {
		return "point_to_point"
	}
	threshold := math.Min(150, math.Max(35, index.Distance*0.025))
	for i := 0; i < len(points)/2; i++ {
		a, b := points[i], points[len(points)-1-i]
		if haversineMeters(a.Latitude, a.Longitude, b.Latitude, b.Longitude) > threshold {
			return ""
		}
	}
	return "out_and_back"
}

func (f routeGroupFacts) ruleTitle(distance float64) string {
	place := f.Scenic
	if place == "" {
		place = f.Start
	}
	if f.Start != "" && f.Finish != "" && f.Start != f.Finish {
		place = f.Start + "—" + f.Finish
	}
	form := "路线"
	switch f.Form {
	case "loop":
		form = "环线"
	case "out_and_back":
		form = "往返"
	}
	if validRouteMetricDistance(distance) {
		var length string
		if distance >= 1000 {
			length = fmt.Sprintf("%g 公里", math.Round(distance/1000*2)/2)
		} else {
			length = fmt.Sprintf("%.0f 米", math.Round(distance/50)*50)
		}
		title := "约 " + length + form
		if place != "" {
			title = place + " · " + title
		}
		return title
	}
	if place != "" {
		return place + form
	}
	return f.TrackType + form
}

func (f routeGroupFacts) introduction(group *models.TrackRouteGroup) *models.TrackRouteIntroduction {
	name := strings.TrimSpace(group.Name)
	if name == "" {
		name = f.ruleTitle(group.Distance)
	}
	shape, shapeEN := "路线", "route"
	switch f.Form {
	case "loop":
		shape, shapeEN = "环线", "loop"
	case "out_and_back":
		shape, shapeEN = "往返路线", "out-and-back route"
	case "point_to_point":
		shape, shapeEN = "单程路线", "point-to-point route"
	}
	zh, en := f.TrackType+shape+"。", "A "+group.TrackType+" "+shapeEN+"."
	if f.Start != "" && f.Finish != "" {
		zh, en = fmt.Sprintf("从%s出发，到%s结束的%s%s。", f.Start, f.Finish, f.TrackType, shape),
			fmt.Sprintf("A %s %s from %s to %s.", group.TrackType, shapeEN, f.Start, f.Finish)
	} else if f.Start != "" {
		zh, en = fmt.Sprintf("从%s出发的%s%s。", f.Start, f.TrackType, shape),
			fmt.Sprintf("A %s %s starting in %s.", group.TrackType, shapeEN, f.Start)
	}
	if f.Scenic != "" && f.Scenic != f.Start && f.Scenic != f.Finish {
		zh += "关联景点为" + f.Scenic + "。"
		en += " Associated place: " + f.Scenic + "."
	}
	return &models.TrackRouteIntroduction{
		Status: models.TrackRouteIntroductionStatusAutomatic, CurrentGroupID: group.GroupID,
		Chinese:              models.TrackRouteIntroductionContent{Name: name, Summary: zh},
		English:              models.TrackRouteIntroductionContent{Name: name, Summary: en},
		EstimatedDurationMin: f.Duration.Low, EstimatedDurationMax: f.Duration.High,
		ObservedDistanceMin: f.Distance.Low, ObservedDistanceMax: f.Distance.High,
		ObservedElevationMin: f.Elevation.Low, ObservedElevationMax: f.Elevation.High,
		ObservedSampleCount: f.SampleCount,
	}
}
