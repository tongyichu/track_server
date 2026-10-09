package main

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/tongyichu/track_server/internal/service"
)

func TestReadPlanRejectsUnsafeChangesBeforeApply(t *testing.T) {
	change := service.TrackTitleChange{TrackID: "NO.00000001", UserID: 7,
		OldTitle: "2026年10月9日 11:08", NewTitle: "西湖公园周边跑步"}
	for _, test := range []struct {
		name string
		edit func(*titlePlan)
	}{
		{"wrong database", func(p *titlePlan) { p.Database = "other" }},
		{"wrong version", func(p *titlePlan) { p.Version = 2 }},
		{"duplicate", func(p *titlePlan) { p.Changes = append(p.Changes, change) }},
		{"custom original", func(p *titlePlan) { p.Changes[0].OldTitle = "我的晨跑路线" }},
		{"empty replacement", func(p *titlePlan) { p.Changes[0].NewTitle = "" }},
		{"date replacement", func(p *titlePlan) { p.Changes[0].NewTitle = "2026-10-09" }},
		{"missing owner", func(p *titlePlan) { p.Changes[0].UserID = 0 }},
	} {
		t.Run(test.name, func(t *testing.T) {
			plan := titlePlan{Version: 1, Database: "target", Changes: []service.TrackTitleChange{change}}
			test.edit(&plan)
			path := writeTestPlan(t, plan)
			if _, err := readPlan(path, "target"); err == nil {
				t.Fatal("unsafe plan accepted")
			}
		})
	}
	plan := titlePlan{Version: 1, Database: "target", Changes: []service.TrackTitleChange{change}}
	if got, err := readPlan(writeTestPlan(t, plan), "target"); err != nil || len(got.Changes) != 1 {
		t.Fatalf("valid plan rejected: %+v %v", got, err)
	}
}

func writeTestPlan(t *testing.T, plan titlePlan) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "plan.json")
	data, err := json.Marshal(plan)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, data, 0o600); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestOutputNeverOverwritesExistingFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "plan.json")
	if err := os.WriteFile(path, []byte("keep"), 0o600); err != nil {
		t.Fatal(err)
	}
	if file, err := createOutput(path); err == nil {
		file.Close()
		t.Fatal("existing file was opened for overwrite")
	}
	data, err := os.ReadFile(path)
	if err != nil || string(data) != "keep" {
		t.Fatalf("file changed: %q %v", data, err)
	}
}

func TestBackfillRequiresExplicitModeAndConnection(t *testing.T) {
	for _, opt := range []options{{}, {planPath: "a", applyPath: "b"}, {applyPath: "b"},
		{applyPath: "b", resultPath: "c", userID: 7}, {planPath: "a", userID: -1}} {
		if err := run(context.Background(), opt); err == nil {
			t.Fatalf("invalid options accepted: %+v", opt)
		}
	}
	t.Setenv("MYSQL_DSN", "")
	t.Setenv("USE_IN_MEMORY_STORE", "true")
	if err := run(context.Background(), options{planPath: "a"}); err == nil {
		t.Fatal("missing real connection accepted")
	}
}
