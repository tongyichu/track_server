package service

import (
	"regexp"
	"strings"
	"testing"
	"time"
)

// TestGenerateTrackID verifies generated ids are opaque, short and unique.
func TestGenerateTrackID(t *testing.T) {
	trackID := generateTrackID()
	println("trackID:" + trackID)
	if !strings.HasPrefix(trackID, "No.") {
		t.Fatalf("expected track id to start with No., got %q", trackID)
	}
	if strings.Count(trackID, ".") != 2 {
		t.Fatalf("expected track id to contain 2 dots (prefix + timestamp), got %q", trackID)
	}

	suffix := strings.TrimPrefix(trackID, "No.")
	if wantLen := len("No.") + len("20060102150405.000000000"); len(trackID) != wantLen {
		t.Fatalf("expected track id length to be %d, got %d (%q)", wantLen, len(trackID), trackID)
	}
	if !regexp.MustCompile(`^\d{14}\.\d{9}$`).MatchString(suffix) {
		t.Fatalf("expected suffix to be timestamp like YYYYMMDDhhmmss.nnnnnnnnn, got %q", suffix)
	}

	seen := map[string]struct{}{trackID: {}}
	for i := 0; i < 200; i++ {
		// Avoid flakiness on platforms with coarse time resolution.
		time.Sleep(time.Microsecond)
		id := generateTrackID()
		if _, ok := seen[id]; ok {
			t.Fatalf("expected generated ids to be unique, got duplicate %q", id)
		}
		seen[id] = struct{}{}
	}
}
