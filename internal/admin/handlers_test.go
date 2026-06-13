package admin

import "testing"

func TestRewriteAdminStaticURL(t *testing.T) {
	got := rewriteAdminStaticURL("/api/v1/static/avatars/1001.png")
	if got != "/admin/api/static/avatars/1001.png" {
		t.Fatalf("unexpected rewritten URL: %q", got)
	}
	external := "https://example.com/avatar.png"
	if got := rewriteAdminStaticURL(external); got != external {
		t.Fatalf("external URL should not be rewritten: %q", got)
	}
}

func TestCleanAdminStaticPath(t *testing.T) {
	tests := []struct {
		name string
		raw  string
		want string
		ok   bool
	}{
		{name: "normal", raw: "/avatars/1001.png", want: "avatars/1001.png", ok: true},
		{name: "traversal", raw: "avatars/../avatars/1001.png", ok: false},
		{name: "empty", raw: "", ok: false},
		{name: "root", raw: "/", ok: false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, ok := cleanAdminStaticPath(tt.raw)
			if ok != tt.ok || got != tt.want {
				t.Fatalf("cleanAdminStaticPath(%q)=%q,%v want %q,%v", tt.raw, got, ok, tt.want, tt.ok)
			}
		})
	}
}

func TestAdminDefaultAvatarPath(t *testing.T) {
	if !isAdminDefaultAvatarPath("default_avatars/girl_01.png") {
		t.Fatal("expected known default avatar path")
	}
	if isAdminDefaultAvatarPath("default_avatars/../../server.log") {
		t.Fatal("unexpected default avatar path match")
	}
}
