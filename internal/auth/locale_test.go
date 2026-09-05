package auth

import (
	"errors"
	"testing"
)

func TestNormalizeLocale(t *testing.T) {
	t.Parallel()
	tests := []struct {
		in      string
		want    string
		wantErr error
	}{
		{"", LocaleEN, nil},
		{"  ", LocaleEN, nil},
		{"en", LocaleEN, nil},
		{"EN", LocaleEN, nil},
		{"en-US", LocaleEN, nil},
		{"en_GB", LocaleEN, nil},
		{"pl", LocalePL, nil},
		{" PL-pl ", LocalePL, nil},
		{"de", "", ErrLocale},
		{"fr-FR", "", ErrLocale},
		{"not-a-locale", "", ErrLocale},
	}
	for _, tc := range tests {
		t.Run(tc.in, func(t *testing.T) {
			got, err := NormalizeLocale(tc.in)
			if tc.wantErr != nil {
				if !errors.Is(err, tc.wantErr) {
					t.Fatalf("NormalizeLocale(%q) = (%q, %v), want %v", tc.in, got, err, tc.wantErr)
				}
				if got != "" {
					t.Errorf("rejected locale returned %q, want empty", got)
				}
				return
			}
			if err != nil || got != tc.want {
				t.Fatalf("NormalizeLocale(%q) = (%q, %v), want (%q, nil)", tc.in, got, err, tc.want)
			}
		})
	}
}
