package models

import "time"

const (
	AnalyticsSyncStatusSuccess = "success"
	AnalyticsSyncStatusPartial = "partial"
	AnalyticsSyncStatusFailed  = "failed"
)

type AnalyticsSyncSummary struct {
	ID            int64     `json:"id" bson:"id"`
	JobName       string    `json:"job_name" bson:"job_name"`
	Status        string    `json:"status" bson:"status"`
	StartedAt     time.Time `json:"started_at" bson:"started_at"`
	EndedAt       time.Time `json:"ended_at" bson:"ended_at"`
	DurationMS    int64     `json:"duration_ms" bson:"duration_ms"`
	ScannedFiles  int       `json:"scanned_files" bson:"scanned_files"`
	UploadedFiles int       `json:"uploaded_files" bson:"uploaded_files"`
	FailedFiles   int       `json:"failed_files" bson:"failed_files"`
	TotalBytes    int64     `json:"total_bytes" bson:"total_bytes"`
	OSSPrefix     string    `json:"oss_prefix" bson:"oss_prefix"`
	FilesJSON     string    `json:"files_json" bson:"files_json"`
	ErrorMessage  string    `json:"error_message" bson:"error_message"`
	CreatedAt     time.Time `json:"created_at" bson:"created_at"`
}

type AnalyticsSyncFileSummary struct {
	LocalPath string `json:"local_path"`
	OSSKey    string `json:"oss_key,omitempty"`
	SizeBytes int64  `json:"size_bytes"`
	Status    string `json:"status"`
	Error     string `json:"error,omitempty"`
}
