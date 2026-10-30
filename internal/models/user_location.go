package models

// UserLocation is the last confirmed city-level location of a user.
type UserLocation struct {
	ProvinceCode string `json:"province_code" bson:"province_code"`
	ProvinceName string `json:"province_name" bson:"province_name"`
	CityCode     string `json:"city_code" bson:"city_code"`
	CityName     string `json:"city_name" bson:"city_name"`
}
