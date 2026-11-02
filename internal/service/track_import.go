package service

import (
	"archive/zip"
	"context"
	"errors"
	"fmt"
	"math"
	"math/rand"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/tongyichu/track_server/internal/config"
	"github.com/tongyichu/track_server/internal/maparea"
	"github.com/tongyichu/track_server/internal/models"
)

// AdminTrackImportService 支持管理后台批量导入轨迹文件并归属给指定内部人员。
//
// 流程与客户端 devmode 批量上传对齐：解析原始轨迹文件（JSON/GeoJSON/KML/KMZ）
// → 计算距离/耗时/起止时间 → 原始文件上传 OSS 归属人目录 → CreateTrack 建档
// （自动触发地图索引排队与成就结算）。预览四图由客户端 devmode 后续补齐，导入不生成。
type AdminTrackImportService struct {
	tracks *TrackService
	oss    *OSSTokenService
}

func NewAdminTrackImportService(tracks *TrackService, oss *OSSTokenService) *AdminTrackImportService {
	return &AdminTrackImportService{tracks: tracks, oss: oss}
}

// ImportTrackFile 是单个待导入的轨迹文件（已落盘到临时目录）。
type ImportTrackFile struct {
	FileName  string
	LocalPath string
}

// ImportTrackResult 是单个文件的导入结果。
type ImportTrackResult struct {
	FileName string `json:"file_name"`
	TrackID  string `json:"track_id,omitempty"`
	Title    string `json:"title,omitempty"`
	Error    string `json:"error,omitempty"`
}

// ImportTracks 逐文件导入，单条失败不影响其他文件。
func (s *AdminTrackImportService) ImportTracks(ctx context.Context, ownerUserID int64, trackType, coordinateSystem string, files []ImportTrackFile) []ImportTrackResult {
	results := make([]ImportTrackResult, 0, len(files))
	for _, file := range files {
		results = append(results, s.importOne(ctx, ownerUserID, trackType, coordinateSystem, file))
	}
	return results
}

func (s *AdminTrackImportService) importOne(ctx context.Context, ownerUserID int64, trackType, coordinateSystem string, file ImportTrackFile) ImportTrackResult {
	result := ImportTrackResult{FileName: file.FileName}
	fail := func(err error) ImportTrackResult {
		result.Error = err.Error()
		return result
	}
	if s == nil || s.tracks == nil || s.oss == nil {
		return fail(errors.New("track import service not configured"))
	}
	if ownerUserID <= 0 {
		return fail(invalidArg("owner_user_id is required"))
	}
	points, err := parseTrackPointsFile(file.LocalPath)
	if err != nil {
		return fail(fmt.Errorf("轨迹文件解析失败: %w", err))
	}
	if len(points) < 2 {
		return fail(errors.New("轨迹点不足 2 个，无法生成轨迹"))
	}
	ensureImportTimestamps(points, trackType)

	distance := importTrackDistance(points)
	startTime, endTime := importTrackTimeRange(points)
	var duration uint32
	if endTime.After(startTime) {
		duration = uint32(endTime.Sub(startTime).Seconds())
	}
	elevationGain := importTrackElevationGain(points)
	var avgSpeed float64
	if duration > 0 {
		avgSpeed = distance / float64(duration) * 3.6
	}

	cityCode, locateAddr := importTrackLocation(points, coordinateSystem)
	titleTrack := &models.Track{TrackType: trackType, Distance: distance, LocateAddr: locateAddr, StartTime: startTime}
	title := generatedTrackTitle(titleTrack, points)
	if nameTitle := importTitleFromFileName(file.FileName); nameTitle != "" {
		title = nameTitle
	}
	if title == "" {
		title = "导入的轨迹"
	}

	// 客户端详情/地图只解析 KMZ（zip 内 gx:Track KML）。已是 KMZ 的原样上传；
	// KML/JSON/GeoJSON 统一转换为 gx:Track KMZ 再上传，保证客户端可绘制。
	uploadPath, uploadName, cleanup, err := prepareImportUpload(file, points)
	if err != nil {
		return fail(fmt.Errorf("转换 KMZ 失败: %w", err))
	}
	defer cleanup()
	rawURL, err := s.oss.UploadUserObject(ownerUserID, uploadName, uploadPath)
	if err != nil {
		return fail(fmt.Errorf("原始轨迹上传 OSS 失败: %w", err))
	}

	isRunning := false
	input := CreateTrackInput{
		Title:            &title,
		CityCode:         &cityCode,
		LocateAddr:       &locateAddr,
		TrackType:        &trackType,
		SourceTag:        stringPtr(trackSourceTagManual),
		CoordinateSystem: &coordinateSystem,
		StartTime:        &startTime,
		EndTime:          &endTime,
		Distance:         &distance,
		Duration:         &duration,
		ElevationGain:    &elevationGain,
		RawTrackURL:      &rawURL,
		IsRunning:        &isRunning,
		AvgSpeedKmh:      &avgSpeed,
	}
	track, err := s.tracks.CreateTrack(ctx, ownerUserID, input)
	if err != nil {
		return fail(fmt.Errorf("创建轨迹失败: %w", err))
	}
	result.TrackID = track.ID
	result.Title = track.Title
	return result
}

func stringPtr(v string) *string { return &v }

// ensureImportTimestamps 在源文件完全不带时间戳时，按运动类型的估算速度
// 沿轨迹累计合成时间戳（起点为当前时间），保证耗时/均速/热量可计算，
// 且转换后的 KMZ 带 <when> 使客户端配速/海拔图表可绘制。已有时间戳的不动。
// 为让客户端图表曲线接近真实记录：逐段速度在基准值 ±25% 内确定性抖动，
// 按坡度爬坡减速/下坡略加速，徒步/爬山每行进 12~20 分钟插入 1~4 分钟休息。
func ensureImportTimestamps(points []models.TrackPoint, trackType string) {
	for _, p := range points {
		if !p.Timestamp.IsZero() {
			return
		}
	}
	code := normalizeTrackTypeCode(trackType)
	baseSpeed := importTrackNominalSpeedKmh(code) / 3.6
	totalDistance := importTrackDistance(points)
	rng := rand.New(rand.NewSource(int64(len(points))*7919 + int64(totalDistance)))

	var moving time.Duration
	nextPause := 12*time.Minute + time.Duration(rng.Float64()*8*float64(time.Minute))
	points[0].Timestamp = time.Now()
	for i := 1; i < len(points); i++ {
		seg := haversineMeters(points[i-1].Latitude, points[i-1].Longitude, points[i].Latitude, points[i].Longitude)
		// 坡度修正：上坡减速，下坡最多加速 30%。
		grade := 0.0
		if seg > 1 {
			grade = (points[i].Elevation - points[i-1].Elevation) / seg
		}
		slopeFactor := 1.0
		if grade > 0 {
			slopeFactor = 1 / (1 + grade*8)
		} else if grade < 0 {
			slopeFactor = 1 + math.Min(-grade*3, 0.3)
		}
		speed := baseSpeed * (0.75 + 0.5*rng.Float64()) * slopeFactor
		segDuration := time.Duration(seg / speed * float64(time.Second))
		points[i].Timestamp = points[i-1].Timestamp.Add(segDuration)
		moving += segDuration
		// 徒步/爬山类低速运动插入休息停顿。
		if (code == "hiking" || code == "climbing") && moving >= nextPause {
			pause := time.Minute + time.Duration(rng.Float64()*3*float64(time.Minute))
			points[i].Timestamp = points[i].Timestamp.Add(pause)
			moving = 0
			nextPause = 12*time.Minute + time.Duration(rng.Float64()*8*float64(time.Minute))
		}
	}
}

// importTrackNominalSpeedKmh 返回各运动类型的估算速度（km/h），仅用于合成时间戳。
func importTrackNominalSpeedKmh(trackType string) float64 {
	switch normalizeTrackTypeCode(trackType) {
	case "running":
		return 10
	case "climbing":
		return 2.5
	case "riding":
		return 15
	case "driving":
		return 60
	default: // hiking 及其他
		return 4
	}
}

// importTrackDistance 用 haversine 累加相邻点距离（米）。
func importTrackDistance(points []models.TrackPoint) float64 {
	var total float64
	for i := 1; i < len(points); i++ {
		total += haversineMeters(points[i-1].Latitude, points[i-1].Longitude, points[i].Latitude, points[i].Longitude)
	}
	return total
}

// importTrackTimeRange 取首尾有效时间戳；无时间信息时以当前时间兜底。
func importTrackTimeRange(points []models.TrackPoint) (time.Time, time.Time) {
	now := time.Now()
	var start, end time.Time
	for _, p := range points {
		if p.Timestamp.IsZero() {
			continue
		}
		if start.IsZero() || p.Timestamp.Before(start) {
			start = p.Timestamp
		}
		if end.IsZero() || p.Timestamp.After(end) {
			end = p.Timestamp
		}
	}
	if start.IsZero() {
		start = now
	}
	if end.IsZero() || end.Before(start) {
		end = start
	}
	return start, end
}

// importTrackElevationGain 累计正向海拔差（米），忽略异常跳变。
func importTrackElevationGain(points []models.TrackPoint) int {
	var gain float64
	for i := 1; i < len(points); i++ {
		delta := points[i].Elevation - points[i-1].Elevation
		if delta > 0 && delta < 500 {
			gain += delta
		}
	}
	return int(math.Round(gain))
}

// importTrackLocation 用内置行政区目录离线解析轨迹中位点所属区县，
// 返回城市 Code 与展示地址；坐标统一先转为 GCJ02。
func importTrackLocation(points []models.TrackPoint, coordinateSystem string) (string, string) {
	mid := points[len(points)/2]
	gcj, err := mapPointToGCJ02(mid, coordinateSystem)
	if err != nil {
		return "", ""
	}
	district := maparea.DefaultCatalog().ResolveDistrict(gcj.Latitude, gcj.Longitude)
	if district == nil {
		return "", ""
	}
	cityName := config.CityNameByCode(district.CityCode)
	districtName := district.Name()
	switch {
	case cityName != "" && districtName != "" && cityName != districtName:
		return district.CityCode, cityName + districtName
	case districtName != "":
		return district.CityCode, districtName
	default:
		return district.CityCode, cityName
	}
}

// prepareImportUpload 决定上传内容：已是 KMZ（zip）时直接复用原文件；
// 否则把解析出的轨迹点写成 gx:Track KML 并打包为 KMZ。
// 返回待上传文件路径、上传文件名与清理函数。
func prepareImportUpload(file ImportTrackFile, points []models.TrackPoint) (string, string, func(), error) {
	base := SanitizeImportFileName(file.FileName)
	data, err := os.ReadFile(file.LocalPath)
	if err != nil {
		return "", "", func() {}, err
	}
	if strings.EqualFold(filepath.Ext(base), ".kmz") || isZipData(data) {
		return file.LocalPath, base, func() {}, nil
	}
	tempDir, err := os.MkdirTemp("", "track-import-kmz-*")
	if err != nil {
		return "", "", func() {}, err
	}
	cleanup := func() { os.RemoveAll(tempDir) }
	ext := filepath.Ext(base)
	name := strings.TrimSuffix(base, ext) + ".kmz"
	kmzPath := filepath.Join(tempDir, name)
	if err := writeImportKMZ(kmzPath, points); err != nil {
		cleanup()
		return "", "", func() {}, err
	}
	return kmzPath, name, cleanup, nil
}

// writeImportKMZ 把轨迹点写成带 gx:Track 的 KML 并打包为 KMZ（客户端渲染所需格式）。
func writeImportKMZ(kmzPath string, points []models.TrackPoint) error {
	var kml strings.Builder
	kml.WriteString("<?xml version=\"1.0\" encoding=\"UTF-8\"?>\n")
	kml.WriteString("<kml xmlns=\"http://www.opengis.net/kml/2.2\" xmlns:gx=\"http://www.google.com/kml/ext/2.2\"><Document><Placemark><gx:Track>\n")
	withTime := true
	for _, p := range points {
		if p.Timestamp.IsZero() {
			withTime = false
			break
		}
	}
	if withTime {
		for _, p := range points {
			kml.WriteString("<when>" + p.Timestamp.UTC().Format(time.RFC3339) + "</when>\n")
		}
	}
	for _, p := range points {
		fmt.Fprintf(&kml, "<gx:coord>%.7f %.7f %.1f</gx:coord>\n", p.Longitude, p.Latitude, p.Elevation)
	}
	kml.WriteString("</gx:Track></Placemark></Document></kml>\n")

	out, err := os.Create(kmzPath)
	if err != nil {
		return err
	}
	zw := zip.NewWriter(out)
	entry, err := zw.Create("doc.kml")
	if err != nil {
		zw.Close()
		out.Close()
		return err
	}
	if _, err := entry.Write([]byte(kml.String())); err != nil {
		zw.Close()
		out.Close()
		return err
	}
	if err := zw.Close(); err != nil {
		out.Close()
		return err
	}
	return out.Close()
}

// importTitleFromFileName 从上传文件名提取默认标题：去扩展名后含汉字时
// 直接作为标题（运营文件命名即路线名，如「北京奥森北园画马2026」），
// 否则返回空串走自动生成规则。最长截取 40 个字符。
func importTitleFromFileName(name string) string {
	base := strings.TrimSuffix(filepath.Base(strings.TrimSpace(name)), filepath.Ext(name))
	base = strings.TrimSpace(base)
	if !hasHanRune(base) {
		return ""
	}
	runes := []rune(base)
	if len(runes) > 40 {
		base = string(runes[:40])
	}
	return base
}

func hasHanRune(s string) bool {
	for _, r := range s {
		if r >= 0x4e00 && r <= 0x9fff {
			return true
		}
	}
	return false
}

// SanitizeImportFileName 清洗上传文件名，仅保留安全字符。
func SanitizeImportFileName(name string) string {
	name = filepath.Base(strings.TrimSpace(name))
	name = strings.Map(func(r rune) rune {
		switch {
		case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r >= '0' && r <= '9',
			r == '.', r == '-', r == '_', r >= 0x4e00 && r <= 0x9fff:
			return r
		}
		return '_'
	}, name)
	if name == "" || name == "." {
		return fmt.Sprintf("track_%d.kml", time.Now().UnixMilli())
	}
	return name
}
