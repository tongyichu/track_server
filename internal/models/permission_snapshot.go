package models

import "time"

type PermissionState struct {
	Key      string `json:"key" bson:"key"`
	Category string `json:"category" bson:"category"`
	State    string `json:"state" bson:"state"`
}

type PermissionSnapshot struct {
	UserID         int64             `json:"-" bson:"user_id"`
	InstallationID string            `json:"installation_id" bson:"installation_id"`
	SchemaVersion  int               `json:"schema_version" bson:"schema_version"`
	Sequence       int64             `json:"sequence" bson:"sequence"`
	Platform       string            `json:"platform" bson:"platform"`
	DeviceModel    string            `json:"device_model" bson:"device_model"`
	OSVersion      string            `json:"os_version" bson:"os_version"`
	AppVersion     string            `json:"app_version" bson:"app_version"`
	VersionCode    int64             `json:"version_code" bson:"version_code"`
	CollectedAt    time.Time         `json:"collected_at" bson:"collected_at"`
	ReceivedAt     time.Time         `json:"received_at" bson:"received_at"`
	Items          []PermissionState `json:"items" bson:"items"`
}
