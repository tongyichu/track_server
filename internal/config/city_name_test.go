package config

import "testing"

func TestCityNameByCode(t *testing.T) {
	tests := []struct {
		name string
		code string
		want string
	}{
		{name: "city code", code: "330100", want: "杭州市"},
		{name: "municipality province code", code: "110000", want: "北京市"},
		{name: "unknown code", code: "000000", want: ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := CityNameByCode(tt.code); got != tt.want {
				t.Fatalf("CityNameByCode(%q) = %q, want %q", tt.code, got, tt.want)
			}
		})
	}
}
