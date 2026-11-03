package service

import (
	"context"
	"errors"
	"strings"

	"github.com/tongyichu/track_server/internal/config"
	"github.com/tongyichu/track_server/internal/models"
)

var ErrInvalidUserLocation = errors.New("city_code must identify a selectable region")

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
	code = config.CanonicalCityCode(code)
	for _, option := range config.CityOptions() {
		if option.Code != code || !option.Selectable {
			continue
		}
		if option.Level == "province" {
			return models.UserLocation{
				ProvinceCode: code,
				ProvinceName: option.Name,
				CityCode:     code,
				CityName:     option.Name,
			}, true
		}
		if option.ProvinceCode != "" {
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
		return models.UserLocation{ProvinceCode: code, ProvinceName: option.Name, CityCode: code, CityName: option.Name}, true
	}
	return models.UserLocation{}, false
}
