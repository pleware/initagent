package hub

import "testing"

func TestParseOrgStatus(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want OrgStatus
		ok   bool
	}{
		{name: "empty is active", in: "", want: OrgStatusActive, ok: true},
		{name: "active", in: "active", want: OrgStatusActive, ok: true},
		{name: "active trimmed and cased", in: " Active ", want: OrgStatusActive, ok: true},
		{name: "suspended", in: "suspended", want: OrgStatusSuspended, ok: true},
		{name: "suspended cased", in: "SUSPENDED", want: OrgStatusSuspended, ok: true},
		{name: "unknown refused", in: "banned", want: OrgStatusActive, ok: false},
		{name: "mode name refused", in: "test", want: OrgStatusActive, ok: false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := ParseOrgStatus(tt.in)
			if tt.ok && err != nil {
				t.Fatalf("ParseOrgStatus(%q) error = %v, want nil", tt.in, err)
			}
			if !tt.ok && err == nil {
				t.Fatalf("ParseOrgStatus(%q) = %q, want an error", tt.in, got)
			}
			if got != tt.want {
				t.Errorf("ParseOrgStatus(%q) = %q, want %q", tt.in, got, tt.want)
			}
		})
	}
}
