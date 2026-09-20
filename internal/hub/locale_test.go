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

func TestListLocales(t *testing.T) {
	locales := listLocales()
	if len(locales) == 0 {
		t.Fatal("listLocales returned an empty catalog")
	}
	if locales[0].Code != "pl" || locales[0].Name != "Polski" {
		t.Errorf("first locale = %+v, want pl/Polski", locales[0])
	}
	byCode := map[string]string{}
	for _, l := range locales {
		byCode[l.Code] = l.Name
	}
	if byCode["en"] != "English" || byCode["de"] != "Deutsch" {
		t.Errorf("locale names = %v, want English and Deutsch present", byCode)
	}
}
