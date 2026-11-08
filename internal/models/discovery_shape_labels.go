package models

type DiscoveryShapeLabelCount struct {
	ShapeLabel string `json:"shape_label" bson:"_id"`
	TrackCount int64  `json:"track_count" bson:"track_count"`
}
