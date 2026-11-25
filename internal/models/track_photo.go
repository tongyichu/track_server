package models

// Photo coordinates describe the shutter position in WGS84. The image is private storage.
type TrackPhoto struct {
	ID              string   `json:"id" bson:"_id"`
	TrackID         string   `json:"track_id" bson:"track_id"`
	Slot            int      `json:"slot" bson:"slot,omitempty"`
	TakenAt         int64    `json:"taken_at_ms" bson:"taken_at_ms"`
	Latitude        *float64 `json:"latitude,omitempty" bson:"latitude,omitempty"`
	Longitude       *float64 `json:"longitude,omitempty" bson:"longitude,omitempty"`
	Description     string   `json:"description,omitempty" bson:"description,omitempty"`
	Status          string   `json:"status" bson:"status"`
	Reason          string   `json:"reason,omitempty" bson:"reason,omitempty"`
	Revision        int64    `json:"revision" bson:"revision"`
	ScenicChoice    string   `json:"scenic_choice,omitempty" bson:"scenic_choice,omitempty"`
	InsideIDs       []string `json:"inside_amap_ids,omitempty" bson:"inside_amap_ids,omitempty"`
	LocationChecked bool     `json:"location_checked,omitempty" bson:"location_checked,omitempty"`
	Digest          string   `json:"digest,omitempty" bson:"digest"`
}
