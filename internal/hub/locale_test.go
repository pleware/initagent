package hub

import "testing"

func TestLocaleDisplayName(t *testing.T) {
	tests := []struct {
		in   string
		want string
	}{
		{"pl", "Polski"},
		{"pl_PL", "Polski"},
		{"pl-PL", "Polski"},
		{"en", "English"},
		{"de", "Deutsch"},
		{"fr", "Français"},
		{"", ""},
	}
	for _, tt := range tests {
		if got := localeDisplayName(tt.in); got != tt.want {
			t.Errorf("localeDisplayName(%q) = %q, want %q", tt.in, got, tt.want)
		}
	}
}
