package service

import (
	"context"
	"errors"
	"strings"

	"github.com/tongyichu/track_server/internal/config"
	"github.com/tongyichu/track_server/internal/models"
)

var ErrInvalidUserLocation = errors.New("city_code must identify a supported mainland China city")

func (s *UserService) UpdateLocation(ctx context.Context, userID int64, cityCode string) error {
	if userID <= 0 {
		return errors.New("userID is required")
	}
	location, ok := userLocationByCityCode(strings.TrimSpace(cityCode))
	if !ok {
		return ErrInvalidUserLocation
	}
	user, err := s.users.FindByID(ctx, userID)
	if err != nil {
		return err
	}
	if user.ProvinceCode == location.ProvinceCode && user.ProvinceName == location.ProvinceName &&
		user.CityCode == location.CityCode && user.CityName == location.CityName {
		return nil
	}
	return s.users.UpdateLocation(ctx, userID, location)
}

func userLocationByCityCode(code string) (models.UserLocation, bool) {
	for _, option := range config.CityOptions() {
		if option.Code != code {
			continue
		}
		if option.Level == "city" && option.ProvinceCode != "" {
			provinceName := config.CityNameByCode(option.ProvinceCode)
			if provinceName == "" {
				return models.UserLocation{}, false
			}
			return models.UserLocation{
				ProvinceCode: option.ProvinceCode,
				ProvinceName: provinceName,
				CityCode:     option.Code,
				CityName:     option.Name,
			}, true
		}
		if option.Level == "province" && isMainlandMunicipalityCode(code) {
			return models.UserLocation{
				ProvinceCode: code,
				ProvinceName: option.Name,
				CityCode:     code,
				CityName:     option.Name,
			}, true
		}
	}
	return models.UserLocation{}, false
}

func isMainlandMunicipalityCode(code string) bool {
	switch code {
	case "110000", "120000", "310000", "500000":
		return true
	default:
		return false
	}
}
