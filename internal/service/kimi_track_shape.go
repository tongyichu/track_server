package service

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"strings"
	"unicode/utf8"

	trackshape "github.com/tongyichu/track_server/skills/track-shape-label"
)

type TrackShapeCheck struct {
	Category    string `json:"category"`
	Candidate   string `json:"candidate"`
	VisualBasis string `json:"visual_basis"`
}
type TrackShapeCandidate struct {
	Label    string `json:"label"`
	Support  string `json:"support"`
	Conflict string `json:"conflict"`
}
type TrackShapeResult struct {
	TaskID            string                `json:"task_id,omitempty"`
	CategoryChecks    []TrackShapeCheck     `json:"category_checks"`
	Candidates        []TrackShapeCandidate `json:"candidates"`
	ShapeLabel        string                `json:"shape_label"`
	RecognitionStatus string                `json:"recognition_status"`
	Appropriateness   string                `json:"appropriateness"`
	DecisionSummary   string                `json:"decision_summary"`
}
type TrackShapeGenerator interface {
	GenerateShape(context.Context, string, string) (*TrackShapeResult, error)
}

func (g *KimiTrackNarrativeGenerator) GenerateShape(ctx context.Context, facts, imagePath string) (*TrackShapeResult, error) {
	text, err := g.complete(ctx, facts, imagePath, trackshape.Prompt+trackshape.ShapeOnlyOutput, true)
	if err != nil {
		return nil, err
	}
	var result TrackShapeResult
	decoder := json.NewDecoder(strings.NewReader(text))
	decoder.DisallowUnknownFields()
	if err = decoder.Decode(&result); err != nil {
		return nil, errors.New("invalid Kimi shape JSON")
	}
	var extra interface{}
	if decoder.Decode(&extra) != io.EOF {
		return nil, errors.New("trailing Kimi shape JSON")
	}
	if err = validateTrackShapeResult(&result); err != nil {
		return nil, err
	}
	return &result, nil
}

func validateTrackShapeResult(r *TrackShapeResult) error {
	invalid := errors.New("invalid Kimi shape output")
	categories := map[string]bool{"动物/生肖": false, "花草": false, "地标建筑": false, "数字": false, "地图": false, "人物头像": false, "节庆生日": false, "组合图案": false}
	if len(r.CategoryChecks) != len(categories) || len(r.Candidates) > 3 {
		return invalid
	}
	for i := range r.CategoryChecks {
		c := &r.CategoryChecks[i]
		c.Category = strings.TrimSpace(c.Category)
		c.Candidate = strings.TrimSpace(c.Candidate)
		seen, ok := categories[c.Category]
		if !ok || seen || strings.TrimSpace(c.VisualBasis) == "" || utf8.RuneCountInString(c.VisualBasis) > 60 {
			return invalid
		}
		if validateTrackShapeLabel(&c.Candidate) != nil {
			return invalid
		}
		categories[c.Category] = true
	}
	r.ShapeLabel = strings.TrimSpace(r.ShapeLabel)
	if validateTrackShapeLabel(&r.ShapeLabel) != nil || strings.TrimSpace(r.DecisionSummary) == "" || utf8.RuneCountInString(r.DecisionSummary) > 160 {
		return invalid
	}
	matched := false
	for i := range r.Candidates {
		c := &r.Candidates[i]
		c.Label = strings.TrimSpace(c.Label)
		if c.Label == "" || validateTrackShapeLabel(&c.Label) != nil || strings.TrimSpace(c.Support) == "" || utf8.RuneCountInString(c.Support) > 100 || utf8.RuneCountInString(c.Conflict) > 100 {
			return invalid
		}
		if c.Label == r.ShapeLabel {
			matched = true
		}
	}
	switch r.RecognitionStatus {
	case "recognized":
		if r.Appropriateness == "appropriate" && (r.ShapeLabel == "" || !matched) {
			return invalid
		}
	case "ambiguous", "no_candidate":
		r.ShapeLabel = ""
	default:
		return invalid
	}
	switch r.Appropriateness {
	case "appropriate":
	case "inappropriate", "undetermined":
		r.ShapeLabel = ""
	default:
		return invalid
	}
	return nil
}
