package admin

import (
	"context"
	"errors"
	"sort"
	"strings"

	"github.com/tongyichu/track_server/internal/config"
	"github.com/tongyichu/track_server/internal/models"
	"github.com/tongyichu/track_server/internal/regions"
)

// 读取一次权威目录，为三种仓储的原始城市计数统一解析归属；不修改用户记录。
func (h *Handler) resolveUserCityStatistics(ctx context.Context, statistics *models.AdminUserDistributionStats) error {
	if h.regionSvc == nil {
		return errors.New("region catalog unavailable")
	}
	catalog, err := h.regionSvc.Read(ctx)
	if err != nil {
		return err
	}
	byID := make(map[string]regions.Node, len(catalog.Nodes))
	byRegionID := make(map[string]regions.Node, len(catalog.Nodes))
	for _, node := range catalog.Nodes {
		byID[node.ID] = node
		if node.RegionID != "" {
			byRegionID[node.RegionID] = node
		}
	}
	merged := make(map[string]models.AdminUserCityDistributionItem, len(statistics.UserCities))
	for _, raw := range statistics.UserCities {
		item := models.AdminUserCityDistributionItem{Name: "未知城市"}
		if canonical, known := config.ResolveCityCode(raw.RegionID); known {
			if node, exists := byRegionID[canonical]; exists {
				item = userCityRegion(node, byID)
			}
		}
		item.Count = merged[item.RegionID].Count + raw.Count
		merged[item.RegionID] = item
	}
	statistics.UserCities = make([]models.AdminUserCityDistributionItem, 0, len(merged))
	for _, item := range merged {
		statistics.UserCities = append(statistics.UserCities, item)
	}
	sort.Slice(statistics.UserCities, func(i, j int) bool {
		left, right := statistics.UserCities[i], statistics.UserCities[j]
		if left.Count != right.Count {
			return left.Count > right.Count
		}
		return left.RegionID < right.RegionID
	})
	return nil
}

func userCityRegion(node regions.Node, byID map[string]regions.Node) models.AdminUserCityDistributionItem {
	item := models.AdminUserCityDistributionItem{RegionID: node.RegionID, Name: node.Name}
	root, province := node, regions.Node{}
	seen := make(map[string]bool)
	for {
		if seen[root.ID] {
			return models.AdminUserCityDistributionItem{Name: "未知城市"}
		}
		seen[root.ID] = true
		if province.ID == "" && root.Level == "province" {
			province = root
		}
		if root.ParentID == "" {
			break
		}
		parent, exists := byID[root.ParentID]
		if !exists {
			return models.AdminUserCityDistributionItem{Name: "未知城市"}
		}
		root = parent
	}
	item.CountryCode = strings.ToUpper(strings.TrimSpace(root.CountryCode))
	if item.CountryCode == "" || root.Level != "country" {
		return models.AdminUserCityDistributionItem{Name: "未知城市"}
	}
	if province.ID != "" {
		item.ProvinceID, item.ProvinceName = province.RegionID, province.Name
		if item.CountryCode == "CN" {
			item.ProvinceMapCode = userProvinceMapCode(province)
		}
	}
	return item
}

func userProvinceMapCode(province regions.Node) string {
	valid := func(code string) bool {
		return len(code) == 6 && code[0] >= '0' && code[0] <= '9' && code[1] >= '0' && code[1] <= '9' && code[2:] == "0000"
	}
	for _, external := range province.ExternalIDs {
		if external.Provider == "amap" && external.Type == "adcode" && valid(external.Value) {
			return external.Value
		}
	}
	if province.CodeSystem == "cn_adcode" && valid(province.Code) {
		return province.Code
	}
	return ""
}
