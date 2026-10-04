package config

import "testing"

func TestRecommendationConfigDefaultsAndBounds(t *testing.T) {
	t.Setenv("USE_IN_MEMORY_STORE", "true")
	t.Setenv("RECOMMENDATION_ENABLED", "")
	t.Setenv("RECOMMENDATION_FEED_SIZE", "999")
	t.Setenv("RECOMMENDATION_CANDIDATE_LIMIT", "10")
	cfg := Load()
	if cfg.RecommendationEnabled {
		t.Fatal("recommendation must default to disabled")
	}
	if cfg.RecommendationFeedSize != 200 {
		t.Fatalf("feed size=%d", cfg.RecommendationFeedSize)
	}
	if cfg.RecommendationCandidateLimit != 200 {
		t.Fatalf("candidate limit=%d", cfg.RecommendationCandidateLimit)
	}
	if cfg.RecommendationFeedTTLMinutes != 60 || cfg.RecommendationTimeoutMillis != 250 {
		t.Fatalf("unexpected defaults: %+v", cfg)
	}
}
