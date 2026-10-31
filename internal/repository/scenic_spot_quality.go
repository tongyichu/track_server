package repository

import (
	"strings"

	"github.com/tongyichu/track_server/internal/models"
	"go.mongodb.org/mongo-driver/bson"
)

// ScenicSpotMissingFields defines the catalog quality checks used by statistics and filtering.
func ScenicSpotMissingFields(spot *models.ScenicSpot) []string {
	missing := make([]string, 0, 4)
	if strings.TrimSpace(spot.Description) == "" {
		missing = append(missing, "description")
	}
	if len(spot.Photos) == 0 {
		missing = append(missing, "photos")
	}
	if !(spot.Latitude >= -90 && spot.Latitude <= 90 && spot.Longitude >= -180 && spot.Longitude <= 180 && spot.Latitude != 0 && spot.Longitude != 0) {
		missing = append(missing, "coordinates")
	}
	if strings.TrimSpace(spot.Address) == "" {
		missing = append(missing, "address")
	}
	return missing
}

func matchesScenicSpotMissingField(spot *models.ScenicSpot, key string) bool {
	if key == "" {
		return true
	}
	for _, field := range ScenicSpotMissingFields(spot) {
		if key == "any" || key == field {
			return true
		}
	}
	return false
}

func scenicSpotMissingSQL(key string) string {
	checks := map[string]string{
		"description": `(COALESCE(JSON_TYPE(JSON_EXTRACT(data_json,'$.description')),'NULL')='NULL' OR JSON_UNQUOTE(JSON_EXTRACT(data_json,'$.description')) REGEXP '^[[:space:]]*$')`,
		"photos":      `(COALESCE(JSON_TYPE(JSON_EXTRACT(data_json,'$.photos')),'NULL')<>'ARRAY' OR COALESCE(JSON_LENGTH(JSON_EXTRACT(data_json,'$.photos')),0)=0)`,
		"coordinates": `NOT (COALESCE(JSON_EXTRACT(data_json,'$.latitude')+0,0) BETWEEN -90 AND 90 AND COALESCE(JSON_EXTRACT(data_json,'$.longitude')+0,0) BETWEEN -180 AND 180 AND COALESCE(JSON_EXTRACT(data_json,'$.latitude')+0,0)<>0 AND COALESCE(JSON_EXTRACT(data_json,'$.longitude')+0,0)<>0)`,
		"address":     `(COALESCE(JSON_TYPE(JSON_EXTRACT(data_json,'$.address')),'NULL')='NULL' OR JSON_UNQUOTE(JSON_EXTRACT(data_json,'$.address')) REGEXP '^[[:space:]]*$')`,
	}
	if key == "any" {
		return "(" + strings.Join([]string{checks["description"], checks["photos"], checks["coordinates"], checks["address"]}, " OR ") + ")"
	}
	if check := checks[key]; check != "" {
		return check
	}
	return "1=0"
}

func scenicSpotMissingMongo(key string) bson.M {
	emptyText := func(field string) bson.M {
		return bson.M{"$or": bson.A{bson.M{field: nil}, bson.M{field: bson.M{"$regex": `^\s*$`}}}}
	}
	checks := map[string]bson.M{
		"description": emptyText("description"),
		"photos":      {"photos.0": bson.M{"$exists": false}},
		"coordinates": {"$nor": bson.A{bson.M{
			"latitude":  bson.M{"$gte": -90, "$lte": 90, "$ne": 0},
			"longitude": bson.M{"$gte": -180, "$lte": 180, "$ne": 0},
		}}},
		"address": emptyText("address"),
	}
	if key == "any" {
		return bson.M{"$or": bson.A{checks["description"], checks["photos"], checks["coordinates"], checks["address"]}}
	}
	if check := checks[key]; check != nil {
		return check
	}
	return bson.M{"_id": bson.M{"$exists": false}}
}
