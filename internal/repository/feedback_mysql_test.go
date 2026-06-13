package repository

import (
	"testing"

	"github.com/tongyichu/track_server/internal/models"
)

func TestFeedbackImagesJSONKeepsStoragePath(t *testing.T) {
	raw, err := marshalFeedbackImagesJSON([]models.FeedbackImage{{
		ImageID:     "1",
		StoragePath: "180888874041528320/20260613/FB202606130720131E104A52_1.jpg",
		MimeType:    "image/jpeg",
		Size:        3494853,
	}})
	if err != nil {
		t.Fatalf("marshalFeedbackImagesJSON: %v", err)
	}

	images, err := unmarshalFeedbackImagesJSON(string(raw))
	if err != nil {
		t.Fatalf("unmarshalFeedbackImagesJSON: %v", err)
	}
	if len(images) != 1 {
		t.Fatalf("expected 1 image, got %d", len(images))
	}
	if images[0].StoragePath != "180888874041528320/20260613/FB202606130720131E104A52_1.jpg" {
		t.Fatalf("storage_path lost: %+v", images[0])
	}
}

func TestFeedbackImagesJSONReadsLegacyRowsWithoutStoragePath(t *testing.T) {
	images, err := unmarshalFeedbackImagesJSON(`[{"size":3494853,"image_id":"1","mime_type":"image/jpeg"}]`)
	if err != nil {
		t.Fatalf("unmarshalFeedbackImagesJSON: %v", err)
	}
	if len(images) != 1 {
		t.Fatalf("expected 1 image, got %d", len(images))
	}
	if images[0].StoragePath != "" {
		t.Fatalf("legacy row should keep empty storage path, got %q", images[0].StoragePath)
	}
}
