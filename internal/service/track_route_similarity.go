package service

import (
	"math"
	"sort"
	"strings"
	"time"

	"github.com/tongyichu/track_server/internal/models"
)

const (
	routeDistanceRatioMin = 0.65
	routeResamplePoints   = 32
)

type routePairEvaluation struct {
	match         bool
	score         float64
	dissimilarity float64
	direction     models.TrackRouteGroupMemberDirection
}

type routePairKey struct{ left, right int }

type routeSimilarityMatrix struct {
	values map[routePairKey]routePairEvaluation
}

func (m routeSimilarityMatrix) get(left, right int) routePairEvaluation {
	if left == right {
		return routePairEvaluation{match: true, score: 1, direction: models.TrackRouteGroupMemberDirectionForward}
	}
	if left > right {
		left, right = right, left
	}
	if value, ok := m.values[routePairKey{left: left, right: right}]; ok {
		return value
	}
	return routePairEvaluation{dissimilarity: 2, direction: models.TrackRouteGroupMemberDirectionForward}
}

func buildRouteSimilarityMatrix(indexes []*models.TrackGeoIndex) routeSimilarityMatrix {
	matrix := routeSimilarityMatrix{values: make(map[routePairKey]routePairEvaluation)}
	for i := 0; i < len(indexes); i++ {
		for j := i + 1; j < len(indexes); j++ {
			if indexes[i].TrackType != indexes[j].TrackType {
				break
			}
			latThreshold := routeGroupCenterThresholdM(indexes[i].TrackType) / 111000.0
			if indexes[j].CenterLat-indexes[i].CenterLat > latThreshold {
				break
			}
			evaluation := evaluateRoutePair(indexes[i], indexes[j])
			if evaluation.match {
				matrix.values[routePairKey{left: i, right: j}] = evaluation
			}
		}
	}
	return matrix
}

func evaluateRoutePair(a, b *models.TrackGeoIndex) routePairEvaluation {
	result := routePairEvaluation{dissimilarity: 2, direction: models.TrackRouteGroupMemberDirectionForward}
	if a == nil || b == nil || strings.TrimSpace(a.TrackType) == "" || a.TrackType != b.TrackType {
		return result
	}
	centerThreshold := routeGroupCenterThresholdM(a.TrackType)
	centerDistance := haversineMeters(a.CenterLat, a.CenterLng, b.CenterLat, b.CenterLng)
	if centerDistance > centerThreshold {
		return result
	}
	maxDistance := math.Max(a.Distance, b.Distance)
	if maxDistance <= 0 {
		return result
	}
	distanceRatio := math.Min(a.Distance, b.Distance) / maxDistance
	if distanceRatio < routeDistanceRatioMin {
		return result
	}
	forwardEndpoint := math.Max(
		haversineMeters(a.StartLat, a.StartLng, b.StartLat, b.StartLng),
		haversineMeters(a.EndLat, a.EndLng, b.EndLat, b.EndLng),
	)
	reverseEndpoint := math.Max(
		haversineMeters(a.StartLat, a.StartLng, b.EndLat, b.EndLng),
		haversineMeters(a.EndLat, a.EndLng, b.StartLat, b.StartLng),
	)
	endpointThreshold := routeEndpointThresholdM(a.TrackType)
	if forwardEndpoint > endpointThreshold && reverseEndpoint > endpointThreshold {
		return result
	}
	polylineA := resampleRoutePolyline(a)
	polylineB := resampleRoutePolyline(b)
	if len(polylineA) < 2 || len(polylineB) < 2 {
		return result
	}
	polylineThreshold := routePolylineThresholdM(a.TrackType)
	forwardPolylineDistance := discreteFrechetMeters(polylineA, polylineB)
	reverseTrackPoints(polylineB)
	reversePolylineDistance := discreteFrechetMeters(polylineA, polylineB)
	forwardValid := forwardEndpoint <= endpointThreshold && forwardPolylineDistance <= polylineThreshold
	reverseValid := reverseEndpoint <= endpointThreshold && reversePolylineDistance <= polylineThreshold
	if !forwardValid && !reverseValid {
		return result
	}
	endpointDistance, polylineDistance := forwardEndpoint, forwardPolylineDistance
	forwardCost := forwardEndpoint/endpointThreshold + forwardPolylineDistance/polylineThreshold
	reverseCost := reverseEndpoint/endpointThreshold + reversePolylineDistance/polylineThreshold
	if reverseValid && (!forwardValid || reverseCost < forwardCost) {
		endpointDistance, polylineDistance = reverseEndpoint, reversePolylineDistance
		result.direction = models.TrackRouteGroupMemberDirectionReverse
	}
	centerScore := 1 - math.Min(centerDistance/centerThreshold, 1)
	endpointScore := 1 - math.Min(endpointDistance/endpointThreshold, 1)
	polylineScore := 1 - math.Min(polylineDistance/polylineThreshold, 1)
	result.match = true
	result.score = clamp01(0.10*centerScore + 0.20*distanceRatio + 0.25*endpointScore + 0.45*polylineScore)
	result.dissimilarity = 1 - result.score
	return result
}

func routeEndpointThresholdM(trackType string) float64 {
	switch strings.TrimSpace(trackType) {
	case "riding":
		return 2500
	case "driving":
		return 6000
	default:
		return 1200
	}
}

func routePolylineThresholdM(trackType string) float64 {
	switch strings.TrimSpace(trackType) {
	case "riding":
		return 1500
	case "driving":
		return 3000
	default:
		return 700
	}
}

func constrainedRouteClusters(indexes []*models.TrackGeoIndex, matrix routeSimilarityMatrix) [][]int {
	adjacency := make([][]int, len(indexes))
	for pair, evaluation := range matrix.values {
		if !evaluation.match {
			continue
		}
		adjacency[pair.left] = append(adjacency[pair.left], pair.right)
		adjacency[pair.right] = append(adjacency[pair.right], pair.left)
	}
	visited := make([]bool, len(indexes))
	clusters := make([][]int, 0)
	for start := range indexes {
		if visited[start] {
			continue
		}
		component := make([]int, 0)
		queue := []int{start}
		visited[start] = true
		for len(queue) > 0 {
			current := queue[0]
			queue = queue[1:]
			component = append(component, current)
			for _, next := range adjacency[current] {
				if !visited[next] {
					visited[next] = true
					queue = append(queue, next)
				}
			}
		}
		clusters = append(clusters, splitComponentByMedoid(component, matrix)...)
	}
	sort.SliceStable(clusters, func(i, j int) bool { return indexes[clusters[i][0]].TrackID < indexes[clusters[j][0]].TrackID })
	return clusters
}

func splitComponentByMedoid(component []int, matrix routeSimilarityMatrix) [][]int {
	remaining := append([]int(nil), component...)
	clusters := make([][]int, 0)
	for len(remaining) > 0 {
		medoid := chooseRouteMedoid(remaining, matrix)
		cluster := []int{medoid}
		next := make([]int, 0, len(remaining)-1)
		for _, candidate := range remaining {
			if candidate == medoid {
				continue
			}
			if matrix.get(medoid, candidate).match {
				cluster = append(cluster, candidate)
			} else {
				next = append(next, candidate)
			}
		}
		sort.Ints(cluster)
		clusters = append(clusters, cluster)
		remaining = next
	}
	return clusters
}

func chooseRouteMedoid(candidates []int, matrix routeSimilarityMatrix) int {
	best, bestCost := candidates[0], math.MaxFloat64
	for _, candidate := range candidates {
		cost := 0.0
		for _, other := range candidates {
			if candidate == other {
				continue
			}
			cost += matrix.get(candidate, other).dissimilarity
		}
		if cost < bestCost || (cost == bestCost && candidate < best) {
			best, bestCost = candidate, cost
		}
	}
	return best
}

func buildRouteGroupFromCluster(indexes []*models.TrackGeoIndex, cluster []int, matrix routeSimilarityMatrix) *routeGroupCluster {
	medoidPosition := chooseRouteMedoid(cluster, matrix)
	medoid := indexes[medoidPosition]
	now := time.Now()
	group := &models.TrackRouteGroup{
		GroupID: routeGroupIDFromTrackID(medoid.TrackID), Name: defaultRouteGroupName(medoid), TrackType: medoid.TrackType,
		Status: models.TrackRouteGroupStatusActive, CoordinateSystem: mapCoordinateSystem(medoid.CoordinateSystem),
		CenterLat: medoid.CenterLat, CenterLng: medoid.CenterLng, Distance: medoid.Distance,
		RepresentativeTrackID: medoid.TrackID, Source: models.TrackRouteGroupSourceAuto, CreatedAt: now, UpdatedAt: now,
	}
	members := make([]*models.TrackRouteGroupMember, 0, len(cluster))
	clusterIndexes := make([]*models.TrackGeoIndex, 0, len(cluster))
	for _, position := range cluster {
		index := indexes[position]
		clusterIndexes = append(clusterIndexes, index)
		evaluation := matrix.get(medoidPosition, position)
		role := models.TrackRouteGroupMemberRoleMember
		if position == medoidPosition {
			role = models.TrackRouteGroupMemberRoleRepresentative
		}
		members = append(members, &models.TrackRouteGroupMember{GroupID: group.GroupID, TrackID: index.TrackID,
			SimilarityScore: evaluation.score, MatchDirection: evaluation.direction, Role: role,
			Source: models.TrackRouteGroupSourceAuto, CreatedAt: now, UpdatedAt: now})
	}
	rebuildMedoidRouteGroup(group, clusterIndexes, medoid)
	return &routeGroupCluster{group: group, members: members, indexes: clusterIndexes}
}

func rebuildMedoidRouteGroup(group *models.TrackRouteGroup, indexes []*models.TrackGeoIndex, medoid *models.TrackGeoIndex) {
	if group == nil || medoid == nil || len(indexes) == 0 {
		return
	}
	group.CenterLat, group.CenterLng, group.Distance = medoid.CenterLat, medoid.CenterLng, medoid.Distance
	group.RepresentativeTrackID = medoid.TrackID
	group.MinLat, group.MinLng, group.MaxLat, group.MaxLng = indexes[0].MinLat, indexes[0].MinLng, indexes[0].MaxLat, indexes[0].MaxLng
	cities := make([]string, 0, len(indexes))
	group.RadiusM = 0
	for _, index := range indexes {
		if index == nil {
			continue
		}
		cities = append(cities, index.CityCode)
		group.MinLat = math.Min(group.MinLat, index.MinLat)
		group.MinLng = math.Min(group.MinLng, index.MinLng)
		group.MaxLat = math.Max(group.MaxLat, index.MaxLat)
		group.MaxLng = math.Max(group.MaxLng, index.MaxLng)
		group.RadiusM = math.Max(group.RadiusM, routeGroupCoverageRadiusM(group.CenterLat, group.CenterLng, index))
	}
	group.CityCodes = compactCityCodes(cities)
	group.MemberCount = int64(len(indexes))
	group.UpdatedAt = time.Now()
}

func resampleRoutePolyline(index *models.TrackGeoIndex) []models.TrackPoint {
	if index == nil {
		return nil
	}
	points := append([]models.TrackPoint(nil), index.SimplifiedPolyline...)
	if len(points) < 2 {
		points = []models.TrackPoint{{Latitude: index.StartLat, Longitude: index.StartLng}, {Latitude: index.EndLat, Longitude: index.EndLng}}
	}
	if len(points) == routeResamplePoints {
		return points
	}
	cumulative := make([]float64, len(points))
	for i := 1; i < len(points); i++ {
		cumulative[i] = cumulative[i-1] + haversineMeters(points[i-1].Latitude, points[i-1].Longitude, points[i].Latitude, points[i].Longitude)
	}
	total := cumulative[len(cumulative)-1]
	if total <= 0 {
		return points
	}
	result := make([]models.TrackPoint, 0, routeResamplePoints)
	segment := 1
	for i := 0; i < routeResamplePoints; i++ {
		target := total * float64(i) / float64(routeResamplePoints-1)
		for segment < len(cumulative)-1 && cumulative[segment] < target {
			segment++
		}
		left, right := segment-1, segment
		span := cumulative[right] - cumulative[left]
		ratio := 0.0
		if span > 0 {
			ratio = (target - cumulative[left]) / span
		}
		result = append(result, models.TrackPoint{Index: i,
			Latitude:  points[left].Latitude + (points[right].Latitude-points[left].Latitude)*ratio,
			Longitude: points[left].Longitude + (points[right].Longitude-points[left].Longitude)*ratio})
	}
	return result
}

func reverseTrackPoints(points []models.TrackPoint) {
	for left, right := 0, len(points)-1; left < right; left, right = left+1, right-1 {
		points[left], points[right] = points[right], points[left]
	}
}

func discreteFrechetMeters(a, b []models.TrackPoint) float64 {
	if len(a) == 0 || len(b) == 0 {
		return math.MaxFloat64
	}
	cache := make([][]float64, len(a))
	for i := range cache {
		cache[i] = make([]float64, len(b))
		for j := range cache[i] {
			cache[i][j] = -1
		}
	}
	var visit func(int, int) float64
	visit = func(i, j int) float64 {
		if cache[i][j] >= 0 {
			return cache[i][j]
		}
		distance := haversineMeters(a[i].Latitude, a[i].Longitude, b[j].Latitude, b[j].Longitude)
		switch {
		case i == 0 && j == 0:
			cache[i][j] = distance
		case i > 0 && j == 0:
			cache[i][j] = math.Max(visit(i-1, 0), distance)
		case i == 0 && j > 0:
			cache[i][j] = math.Max(visit(0, j-1), distance)
		default:
			cache[i][j] = math.Max(math.Min(visit(i-1, j), math.Min(visit(i-1, j-1), visit(i, j-1))), distance)
		}
		return cache[i][j]
	}
	return visit(len(a)-1, len(b)-1)
}

func clamp01(value float64) float64 { return math.Max(0, math.Min(value, 1)) }
