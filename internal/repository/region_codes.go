package repository

import "github.com/tongyichu/track_server/internal/config"

func sameRegionCode(a, b string) bool {
	return config.CanonicalCityCode(a) == config.CanonicalCityCode(b)
}

func regionCodeCandidates(codes []string) []string {
	seen := make(map[string]bool, len(codes)*2)
	result := make([]string, 0, len(codes)*2)
	for _, code := range codes {
		for _, candidate := range config.CityCodeCandidates(code) {
			if !seen[candidate] {
				seen[candidate] = true
				result = append(result, candidate)
			}
		}
	}
	return result
}
