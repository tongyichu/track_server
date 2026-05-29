package config

import "testing"

func TestMatchSensitiveHit(t *testing.T) {
	matched, hit := MatchSensitive("this is a badword in content")
	if !hit {
		t.Fatalf("expected sensitive word to be matched")
	}
	if matched != "badword" {
		t.Fatalf("expected matched=%q, got %q", "badword", matched)
	}
}

func TestMatchSensitiveCaseInsensitive(t *testing.T) {
	matched, hit := MatchSensitive("HELLO BadWord World")
	if !hit {
		t.Fatalf("expected case-insensitive match")
	}
	if matched != "badword" {
		t.Fatalf("expected matched=%q, got %q", "badword", matched)
	}
}

func TestMatchSensitiveMiss(t *testing.T) {
	if matched, hit := MatchSensitive("just a clean message"); hit {
		t.Fatalf("expected no match, got %q", matched)
	}
}

func TestMatchSensitiveEmpty(t *testing.T) {
	if matched, hit := MatchSensitive(""); hit {
		t.Fatalf("expected no match for empty content, got %q", matched)
	}
}

func TestMatchSensitiveChinese(t *testing.T) {
	matched, hit := MatchSensitive("含有敏感词占位1的内容")
	if !hit {
		t.Fatalf("expected chinese sensitive word match")
	}
	if matched != "敏感词占位1" {
		t.Fatalf("expected matched=%q, got %q", "敏感词占位1", matched)
	}
}
