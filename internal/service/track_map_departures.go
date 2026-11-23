package service

import (
	"context"
	"math"
	"slices"
	"sort"
	"strings"
	"time"

	"github.com/tongyichu/track_server/internal/config"
	"github.com/tongyichu/track_server/internal/maparea"
	"github.com/tongyichu/track_server/internal/models"
)

const (
	trackMapBrowseDeparture = "departure"
	trackMapViewDistrict    = "district"
	trackMapViewPlaces      = "places"
	trackMapPlacesMaxZoom   = 13
)

type mapDeparture struct {
	models.TrackMapDeparture
	cityCode     string
	cityName     string
	districtID   string
	districtName string
	scenic       *departureScenicMatch
}

type departureScenicMatch struct {
	id        string
	name      string
	provider  string
	cityCode  string
	point     *models.TrackMapPoint
	catalog   bool
	rank      int
	updatedAt time.Time
}

func (d mapDeparture) scopeID() string {
	if d.districtID != "" {
		return d.districtID
	}
	return "city:" + d.cityCode
}

type departureCluster struct {
	item          *models.TrackMapClusterItem
	groups        map[string]struct{}
	anchor        *models.TrackMapPoint
	anchorTrackID string
}

type TrackMapScenicTracksInput struct {
	ScenicID, ScopeID, TrackType, BBox, Cursor string
	Limit                                      int
}

func (c *departureCluster) add(d mapDeparture) {
	item := c.item
	if c.anchorTrackID != "" && item.RouteCount != 0 {
		c.includeDeparture(d.Start)
	}
	if _, found := c.groups[d.GroupID]; found {
		return
	}
	c.groups[d.GroupID] = struct{}{}
	if c.anchor != nil {
		item.Center = *c.anchor
		if c.anchorTrackID == "" || item.RouteCount == 0 {
			item.BBox = departurePointBounds(*c.anchor)
		}
		item.RouteCount++
		return
	}
	n := float64(item.RouteCount)
	item.Center.Latitude = (item.Center.Latitude*n + d.Start.Latitude) / (n + 1)
	item.Center.Longitude = (item.Center.Longitude*n + d.Start.Longitude) / (n + 1)
	if item.RouteCount == 0 {
		item.BBox = departurePointBounds(d.Start)
	}
	item.RouteCount++
	c.includeDeparture(d.Start)
}

func (c *departureCluster) includeDeparture(start models.TrackMapPoint) {
	bounds := &c.item.BBox
	bounds.MinLatitude = min(bounds.MinLatitude, start.Latitude)
	bounds.MinLongitude = min(bounds.MinLongitude, start.Longitude)
	bounds.MaxLatitude = max(bounds.MaxLatitude, start.Latitude)
	bounds.MaxLongitude = max(bounds.MaxLongitude, start.Longitude)
}

func (s *TrackMapService) departureView(ctx context.Context, input TrackMapViewInput, filter models.TrackMapQueryFilter, legacyLevel string) (*models.TrackMapViewResponse, error) {
	districtID := strings.TrimSpace(input.DistrictID)
	scenicID := strings.TrimSpace(input.ScenicID)
	if len(scenicID) > 128 || scenicID != "" && districtID == "" && filter.CityCode == "" {
		return nil, invalidArg("scenic_id requires a valid city_code or district_id")
	}
	query, cityCode, err := s.departureQuery(filter, districtID)
	if err != nil {
		return nil, err
	}
	level := legacyLevel
	switch {
	case scenicID != "":
		level = trackMapViewRoute
	case districtID != "":
		level = trackMapViewPlaces
	case legacyLevel == trackMapViewArea || cityCode != "" && input.Zoom <= 0:
		level = trackMapViewDistrict
	case legacyLevel == trackMapViewRoute && input.ScenicPins,
		input.Zoom > trackMapAreaMaxZoom && input.Zoom <= trackMapPlacesMaxZoom:
		level = trackMapViewPlaces
	}
	response := &models.TrackMapViewResponse{
		ViewLevel: level, CoordinateSystem: "GCJ02", BrowseMode: trackMapBrowseDeparture, Items: []any{},
	}
	departures := make(map[[2]string]mapDeparture)
	clusters := make(map[string]*departureCluster)
	fixedScenicPins := input.ScenicPins && (level == trackMapViewPlaces || level == trackMapViewDistrict && cityCode != "")
	var scenicCandidates []mapDeparture
	addDeparture := func(d mapDeparture) {
		key := [2]string{d.GroupID, d.scopeID()}
		if previous, exists := departures[key]; !exists || preferScenicDeparture(d, previous) {
			departures[key] = d
		}
	}
	addScenic := func(d mapDeparture) {
		addDeparture(d)
		spot := d.scenic
		if spot.point != nil && !departurePointVisible(*spot.point, query) {
			return
		}
		key := "scenic_" + spot.id
		cluster := clusters[key]
		if cluster == nil {
			item := s.departureScenic(d)
			item.ClusterID = key
			if spot.catalog {
				item.DepartureScopeID = scenicWholeSpotScope
				if spot.cityCode != "" {
					item.CityCode = spot.cityCode
					item.CityName = config.CityNameByCode(spot.cityCode)
				}
			}
			cluster = &departureCluster{item: item, anchor: spot.point, groups: make(map[string]struct{})}
			clusters[key] = cluster
		}
		if spot.point == nil && (cluster.anchorTrackID == "" || d.TrackID < cluster.anchorTrackID) {
			// A real departure stays visible when zooming around this pin, even without catalog coordinates.
			cluster.anchorTrackID = d.TrackID
			cluster.anchor = &d.Start
			cluster.item.Center = d.Start
			if !spot.catalog {
				cluster.item.DepartureScopeID = d.scopeID()
				cluster.item.CityCode, cluster.item.CityName = d.cityCode, d.cityName
			}
		}
		cluster.add(d)
	}
	err = s.visitMapDepartures(ctx, query, cityCode, districtID, func(d mapDeparture) {
		if scenicID != "" || level == trackMapViewPlaces || level == trackMapViewDistrict && cityCode != "" && d.districtID == "" {
			scenicCandidates = append(scenicCandidates, d)
			return
		}
		addDeparture(d)
	})
	if err != nil {
		return nil, err
	}

	scenicMatches, err := s.departureScenicMatches(ctx, scenicCandidates)
	if err != nil {
		return nil, err
	}
	for _, d := range scenicCandidates {
		d.scenic = scenicMatches[d.TrackID]
		if fixedScenicPins && d.scenic != nil {
			addScenic(d)
			continue
		}
		addDeparture(d)
	}
	if fixedScenicPins {
		err := s.visitVisibleScenicPins(ctx, query, func(d mapDeparture) {
			if (cityCode == "" || cityCode == d.cityCode) && (districtID == "" || districtID == d.districtID) &&
				(level == trackMapViewPlaces || d.districtID == "") {
				addScenic(d)
			}
		})
		if err != nil {
			return nil, err
		}
	}
	ordered := make([]mapDeparture, 0, len(departures))
	for _, d := range departures {
		if scenicID == "" || d.scenic != nil && d.scenic.id == scenicID {
			ordered = append(ordered, d)
		}
	}
	sort.Slice(ordered, func(i, j int) bool { return preferDeparture(ordered[i], ordered[j]) })

	routes := make([]mapDeparture, 0, filter.Limit)
	for _, d := range ordered {
		if fixedScenicPins && d.scenic != nil {
			continue
		}
		cluster := s.departureClusterItem(d, level, cityCode)
		if cluster == nil {
			if len(routes) < filter.Limit {
				routes = append(routes, d)
			}
			continue
		}
		acc := clusters[cluster.ClusterID]
		if acc == nil {
			acc = &departureCluster{item: cluster, groups: make(map[string]struct{})}
			clusters[cluster.ClusterID] = acc
		}
		acc.add(d)
	}
	clusterItems := make([]*models.TrackMapClusterItem, 0, len(clusters))
	for _, cluster := range clusters {
		clusterItems = append(clusterItems, cluster.item)
	}
	sort.Slice(clusterItems, func(i, j int) bool {
		a, b := clusterItems[i], clusterItems[j]
		if a.RouteCount != b.RouteCount {
			return a.RouteCount > b.RouteCount
		}
		return a.ClusterID < b.ClusterID
	})
	items := make([]any, 0, min(filter.Limit, len(clusterItems)+len(routes)))
	for _, cluster := range clusterItems[:min(filter.Limit, len(clusterItems))] {
		items = append(items, cluster)
	}
	if routeLimit := min(filter.Limit-len(items), len(routes)); routeLimit > 0 {
		routeItems, err := s.departureRouteItems(ctx, routes[:routeLimit])
		if err != nil {
			return nil, err
		}
		for _, item := range routeItems {
			items = append(items, item)
		}
	}
	response.Items = items
	return response, nil
}

func (s *TrackMapService) departureQuery(filter models.TrackMapQueryFilter, districtID string) (models.TrackMapQueryFilter, string, error) {
	cityCode := filter.CityCode
	var bounds *models.TrackMapBBox
	if districtID != "" {
		district := s.areas.Find(districtID)
		if district == nil || !isDepartureDistrict(district) {
			return filter, "", invalidArg("unknown district_id")
		}
		districtCity := config.CanonicalCityCode(district.CityCode)
		if cityCode != "" && districtCity != cityCode {
			return filter, "", invalidArg("district_id does not belong to city_code")
		}
		cityCode = districtCity
		bounds = departureMapBounds(district.Bounds)
	} else if cityCode != "" {
		if b, ok := s.areas.DistrictCityBounds(config.CityCodeCandidates(cityCode)); ok {
			bounds = departureMapBounds(b)
		}
	}
	// The geographic envelope is a prefilter; city assignment comes from the start point.
	if bounds != nil {
		filter.CityCode = ""
	}
	if filter.Center != nil {
		radiusBounds := departureRadiusBounds(*filter.Center, filter.RadiusM)
		filter.BBox = intersectDepartureBounds(filter.BBox, &radiusBounds)
	}
	filter.BBox = intersectDepartureBounds(filter.BBox, bounds)
	return filter, cityCode, nil
}

func (s *TrackMapService) visitMapDepartures(ctx context.Context, query models.TrackMapQueryFilter, cityCode, districtID string, visit func(mapDeparture)) error {
	if b := query.BBox; b != nil && (b.MinLatitude > b.MaxLatitude || b.MinLongitude > b.MaxLongitude) {
		return nil
	}
	return s.maps.VisitRouteDepartures(ctx, query, func(row models.TrackMapDeparture) {
		if !validMapLatLng(row.Start.Latitude, row.Start.Longitude) ||
			(query.Center != nil && haversineDistance(query.Center.Latitude, query.Center.Longitude, row.Start.Latitude, row.Start.Longitude) > float64(query.RadiusM)) {
			return
		}
		d := s.locateDeparture(row)
		if (cityCode == "" || d.cityCode == cityCode) && (districtID == "" || d.districtID == districtID) {
			visit(d)
		}
	})
}

func (s *TrackMapService) ListScenicTracks(ctx context.Context, userID int64, selection TrackMapScenicTracksInput) (*models.TrackSummaryPage, error) {
	scenicID := strings.TrimSpace(selection.ScenicID)
	scopeID := strings.TrimSpace(selection.ScopeID)
	cursor := strings.TrimSpace(selection.Cursor)
	if scenicID == "" || len(scenicID) > 128 || scopeID == "" || len(scopeID) > 128 || len(cursor) > 32 {
		return nil, invalidArg("invalid scenic_id, departure_scope_id or cursor")
	}
	if scopeID == scenicWholeSpotScope {
		return s.listWholeScenicSpotTracks(ctx, userID, scenicID, selection.TrackType, cursor, selection.Limit)
	}
	input := TrackMapViewInput{BrowseMode: trackMapBrowseDeparture, TrackType: selection.TrackType}
	if cityCode, isCity := strings.CutPrefix(scopeID, "city:"); isCity {
		input.CityCode = cityCode
		if cityCode == "" {
			input.BBox = selection.BBox
		}
	} else {
		input.DistrictID = scopeID
	}
	filter, _, err := s.buildFilter(input)
	if err != nil {
		return nil, err
	}
	query, cityCode, err := s.departureQuery(filter, input.DistrictID)
	if err != nil {
		return nil, err
	}
	var candidates []mapDeparture
	err = s.visitMapDepartures(ctx, query, cityCode, input.DistrictID, func(d mapDeparture) {
		if d.scopeID() == scopeID && d.TrackID > cursor {
			candidates = append(candidates, d)
		}
	})
	if err != nil {
		return nil, err
	}
	sort.Slice(candidates, func(i, j int) bool { return candidates[i].TrackID < candidates[j].TrackID })
	limit := normalizeTrackPageLimit(selection.Limit)
	tracks := make([]*models.Track, 0, limit+1)
	for start := 0; start < len(candidates) && len(tracks) <= limit; start += 200 {
		batch := candidates[start:min(start+200, len(candidates))]
		matches, err := s.departureScenicMatches(ctx, batch)
		if err != nil {
			return nil, err
		}
		ids := make([]string, 0, len(matches))
		for _, departure := range batch {
			if spot := matches[departure.TrackID]; spot != nil && spot.id == scenicID {
				ids = append(ids, departure.TrackID)
			}
		}
		if len(ids) == 0 {
			continue
		}
		byID, err := s.tracks.FindByIDs(ctx, ids)
		if err != nil {
			return nil, err
		}
		for _, id := range ids {
			if track := byID[id]; publicMapTrack(track) && track.RawTrackURL != "" {
				tracks = append(tracks, track)
				if len(tracks) > limit {
					break
				}
			}
		}
	}
	return s.scenicTracksPage(ctx, userID, tracks, limit)
}

func (s *TrackMapService) scenicTracksPage(ctx context.Context, userID int64, tracks []*models.Track, limit int) (*models.TrackSummaryPage, error) {
	page := &models.TrackSummaryPage{HasMore: len(tracks) > limit}
	if page.HasMore {
		tracks = tracks[:limit]
		page.NextCursor = tracks[limit-1].ID
	}
	if s.trackSvc == nil {
		page.Items = toSummaries(tracks)
		return page, nil
	}
	var err error
	page.Items, err = s.trackSvc.BuildTrackSummaries(ctx, userID, tracks)
	return page, err
}

func isDepartureDistrict(district *maparea.Definition) bool {
	return district.Type == maparea.TypeDistrict && district.ID != "district-"+district.CityCode
}

func (s *TrackMapService) locateDeparture(row models.TrackMapDeparture) mapDeparture {
	d := mapDeparture{TrackMapDeparture: row, cityCode: config.CanonicalCityCode(row.CityCode)}
	if area := s.areas.ResolveDistrict(row.Start.Latitude, row.Start.Longitude); area != nil {
		d.cityCode = config.CanonicalCityCode(area.CityCode)
		if isDepartureDistrict(area) {
			d.districtID, d.districtName = area.ID, area.Name()
		} else {
			d.cityName = area.Name()
		}
	}
	if name := config.CityNameByCode(d.cityCode); name != "" {
		d.cityName = name
	}
	return d
}

func preferDeparture(a, b mapDeparture) bool {
	if a.MemberCount != b.MemberCount {
		return a.MemberCount > b.MemberCount
	}
	if a.GroupID != b.GroupID {
		return a.GroupID < b.GroupID
	}
	aRep, bRep := a.TrackID == a.RepresentativeTrackID, b.TrackID == b.RepresentativeTrackID
	if aRep != bRep {
		return aRep
	}
	return a.TrackID < b.TrackID
}

func preferScenicDeparture(a, b mapDeparture) bool {
	switch {
	case a.scenic != nil && b.scenic == nil:
		return true
	case a.scenic == nil && b.scenic != nil:
		return false
	case a.scenic != nil && b.scenic != nil:
		if a.scenic.rank != b.scenic.rank {
			return a.scenic.rank > b.scenic.rank
		}
		if a.scenic.rank == departureScenicManual && !a.scenic.updatedAt.Equal(b.scenic.updatedAt) {
			return a.scenic.updatedAt.After(b.scenic.updatedAt)
		}
	}
	return preferDeparture(a, b)
}

const (
	departureScenicLegacy = iota + 1
	departureScenicAutomatic
	departureScenicManual
)

func (s *TrackMapService) departureScenicMatches(ctx context.Context, departures []mapDeparture) (map[string]*departureScenicMatch, error) {
	matches := make(map[string]*departureScenicMatch)
	if len(departures) == 0 {
		return matches, nil
	}
	trackIDs := make([]string, 0, len(departures))
	for _, d := range departures {
		trackIDs = append(trackIDs, d.TrackID)
	}
	slices.Sort(trackIDs)
	trackIDs = slices.Compact(trackIDs)

	var catalog *ScenicSpotCatalogService
	if s.trackSvc != nil {
		catalog = s.trackSvc.scenicCatalog
	}
	var legacyIDs []string
	for start := 0; start < len(trackIDs); start += 200 {
		batch := trackIDs[start:min(start+200, len(trackIDs))]
		links, spots, err := catalog.primarySpotRecords(ctx, batch)
		if err != nil {
			return nil, err
		}
		for _, id := range batch {
			link := links[id]
			if link == nil {
				legacyIDs = append(legacyIDs, id)
				continue
			}
			// An explicit unassignment or an inactive catalog entry takes precedence over older recognition results.
			matches[id] = nil
			if spot := spots[link.SpotID]; scenicSpotClientVisible(spot) && strings.TrimSpace(spot.Name) != "" {
				rank := departureScenicAutomatic
				if link.Origin == "manual" || link.Origin == "merge" {
					rank = departureScenicManual
				}
				match := catalogScenicMapMatch(spot)
				match.provider, match.rank, match.updatedAt = link.Provider, rank, link.UpdatedAt
				matches[id] = match
			}
		}
	}
	legacy, err := s.places.PrimaryScenicSpots(ctx, legacyIDs)
	if err != nil {
		return nil, err
	}
	for id, spot := range legacy {
		matches[id] = &departureScenicMatch{id: spot.ID, name: spot.Name, provider: spot.Provider, rank: departureScenicLegacy}
	}
	if !s.places.Enabled() {
		for _, d := range departures {
			if _, resolved := matches[d.TrackID]; resolved {
				continue
			}
			if area := s.areas.Find(d.AreaID); area != nil && area.Type == maparea.TypeScenicSpot {
				matches[d.TrackID] = &departureScenicMatch{id: area.ID, name: area.Name(), rank: departureScenicLegacy}
			}
		}
	}
	return matches, nil
}

func (s *TrackMapService) departureClusterItem(d mapDeparture, level, cityCode string) *models.TrackMapClusterItem {
	switch {
	case (level == trackMapViewCity || level == trackMapViewDistrict && cityCode == "" && d.districtID == "") && d.cityCode != "":
		return &models.TrackMapClusterItem{
			Type: "city_cluster", ClusterID: "city_" + d.cityCode, Name: d.cityName, CityCode: d.cityCode, CityName: d.cityName,
		}
	case level == trackMapViewDistrict && d.districtID != "":
		return &models.TrackMapClusterItem{
			Type: "area_cluster", ClusterID: "district_" + d.districtID, Name: d.districtName, AreaType: maparea.TypeDistrict,
			CityCode: d.cityCode, CityName: d.cityName, Area: &models.TrackMapAreaReference{ID: d.districtID},
		}
	case level == trackMapViewPlaces || level == trackMapViewDistrict && cityCode != "":
		if d.scenic != nil {
			return s.departureScenic(d)
		}
	}
	return nil
}

func (s *TrackMapService) departureScenic(d mapDeparture) *models.TrackMapClusterItem {
	spot := d.scenic
	item := &models.TrackMapClusterItem{
		Type: "area_cluster", ClusterID: "scenic_" + spot.id + "_" + d.scopeID(), AreaType: maparea.TypeScenicSpot,
		Area: &models.TrackMapAreaReference{ID: spot.id}, Name: spot.name,
		CityCode: d.cityCode, CityName: d.cityName, PlaceProvider: spot.provider, DepartureScopeID: d.scopeID(),
	}
	if spot.provider != "" {
		item.PlaceMatchMethod = "track_relation"
	}
	if area := s.areas.Find(spot.id); area != nil && area.HasIntroduction() {
		item.Area.IntroductionURL = mapAreaIntroductionURL(spot.id, area.ContentVersion)
	}
	return item
}

func (s *TrackMapService) departureRouteItems(ctx context.Context, routes []mapDeparture) ([]*models.TrackMapRouteGroupItem, error) {
	trackIDs, groupIDs := make([]string, 0, len(routes)), make([]string, 0, len(routes))
	for _, d := range routes {
		trackIDs, groupIDs = append(trackIDs, d.TrackID), append(groupIDs, d.GroupID)
	}
	tracks, err := s.tracks.FindByIDs(ctx, trackIDs)
	if err != nil {
		return nil, err
	}
	introductions, err := s.maps.ListPublishedRouteIntroductions(ctx, groupIDs)
	if err != nil {
		return nil, err
	}
	items := make([]*models.TrackMapRouteGroupItem, 0, len(routes))
	for _, d := range routes {
		track := tracks[d.TrackID]
		if !publicMapTrack(track) {
			continue
		}
		item := &models.TrackMapRouteGroupItem{
			Type: "route_group", GroupID: d.GroupID, Name: d.GroupName, TrackType: d.TrackType,
			CoordinateSystem: "GCJ02", Center: d.Start, BBox: departurePointBounds(d.Start),
			CityCode: d.cityCode, CityName: d.cityName, DistrictName: d.districtName,
			DistanceM: d.Distance, DepartureTrackID: d.TrackID, DepartureScopeID: d.scopeID(),
			CoverTrack: &models.TrackMapCoverTrack{TrackID: track.ID, PreviewImages: trackPreviewURLs(track.ID, track.PreviewImages)},
		}
		if item.Name == "" {
			item.Name = d.cityName + trackTypeDisplayName(d.TrackType) + "路线"
		}
		if end := s.areas.ResolveDistrict(d.End.Latitude, d.End.Longitude); end != nil &&
			(end.ID != d.districtID && (isDepartureDistrict(end) || config.CanonicalCityCode(end.CityCode) != d.cityCode)) {
			item.ArrivalName = end.Name()
			if endCity := config.CanonicalCityCode(end.CityCode); endCity != d.cityCode && isDepartureDistrict(end) {
				if name := config.CityNameByCode(endCity); name != "" {
					item.ArrivalName = name + " " + item.ArrivalName
				}
			}
		}
		if intro := introductions[d.GroupID]; intro != nil {
			item.IntroductionURL = mapRouteIntroductionURL(d.GroupID, intro.ContentVersion)
		}
		items = append(items, item)
	}
	return items, nil
}

func publicMapTrack(track *models.Track) bool {
	return track != nil && track.Status == models.TrackStatusNormal && !track.IsRunning && track.DeletedAt.IsZero()
}

func departureMapBounds(b maparea.Bounds) *models.TrackMapBBox {
	return &models.TrackMapBBox{
		MinLatitude: b.MinLatitude, MinLongitude: b.MinLongitude, MaxLatitude: b.MaxLatitude, MaxLongitude: b.MaxLongitude,
	}
}

func departurePointBounds(p models.TrackMapPoint) models.TrackMapBBox {
	return models.TrackMapBBox{
		MinLatitude: p.Latitude, MinLongitude: p.Longitude, MaxLatitude: p.Latitude, MaxLongitude: p.Longitude,
	}
}

func intersectDepartureBounds(a, b *models.TrackMapBBox) *models.TrackMapBBox {
	if a == nil {
		return b
	}
	if b == nil {
		return a
	}
	return &models.TrackMapBBox{
		MinLatitude: max(a.MinLatitude, b.MinLatitude), MinLongitude: max(a.MinLongitude, b.MinLongitude),
		MaxLatitude: min(a.MaxLatitude, b.MaxLatitude), MaxLongitude: min(a.MaxLongitude, b.MaxLongitude),
	}
}

func departureRadiusBounds(p models.TrackMapPoint, radiusM int) models.TrackMapBBox {
	latSpan := float64(radiusM) / 110000
	south, north := max(-90, p.Latitude-latSpan), min(90, p.Latitude+latSpan)
	lngSpan := latSpan / math.Cos(max(math.Abs(south), math.Abs(north))*math.Pi/180)
	west, east := p.Longitude-lngSpan, p.Longitude+lngSpan
	if west < -180 || east > 180 {
		west, east = -180, 180
	}
	return models.TrackMapBBox{MinLatitude: south, MaxLatitude: north, MinLongitude: west, MaxLongitude: east}
}
