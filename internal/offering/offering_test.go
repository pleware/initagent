package offering

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/pleware/initagent/internal/brand"
)

func TestParse(t *testing.T) {
	t.Parallel()
	cases := []struct {
		in      string
		want    Kind
		wantErr bool
	}{
		{"selfhost", Selfhost, false},
		{" hosted\n", Hosted, false},
		{"HOSTED", Hosted, false},
		{"", "", true},
		{"   ", "", true},
		{"cloud", "", true},
	}
	for _, tc := range cases {
		t.Run(tc.in, func(t *testing.T) {
			t.Parallel()
			got, err := Parse(tc.in)
			if tc.wantErr {
				if err == nil {
					t.Fatalf("Parse(%q) = %q, want error", tc.in, got)
				}
				return
			}
			if err != nil {
				t.Fatalf("Parse(%q): %v", tc.in, err)
			}
			if got != tc.want {
				t.Fatalf("Parse(%q) = %q, want %q", tc.in, got, tc.want)
			}
		})
	}
}

func TestResolve(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name    string
		flag    string
		env     string
		file    string
		present bool
		want    Kind
		wantErr bool
	}{
		{"default missing file", "", "", "", false, Selfhost, false},
		{"file hosted", "", "", "hosted", true, Hosted, false},
		{"env beats file", "", "selfhost", "hosted", true, Selfhost, false},
		{"flag beats env", "hosted", "selfhost", "selfhost", true, Hosted, false},
		{"empty present file", "", "", "  ", true, "", true},
		{"invalid file", "", "", "saas", true, "", true},
		{"invalid flag", "nope", "", "", false, "", true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got, err := Resolve(tc.flag, tc.env, tc.file, tc.present)
			if tc.wantErr {
				if err == nil {
					t.Fatalf("Resolve = %q, want error", got)
				}
				return
			}
			if err != nil {
				t.Fatalf("Resolve: %v", err)
			}
			if got != tc.want {
				t.Fatalf("Resolve = %q, want %q", got, tc.want)
			}
		})
	}
}

func TestRequireStart(t *testing.T) {
	t.Parallel()
	if err := RequireStart(Selfhost, ""); err != nil {
		t.Fatalf("selfhost without Postgres: %v", err)
	}
	if err := RequireStart(Hosted, ""); err == nil {
		t.Fatal("hosted without Postgres: want error")
	}
	if err := RequireStart(Hosted, "  "); err == nil {
		t.Fatal("hosted with blank Postgres: want error")
	}
	if err := RequireStart(Hosted, "postgres://hub"); err != nil {
		t.Fatalf("hosted with Postgres: %v", err)
	}
	err := RequireStart(Hosted, "")
	if err == nil || !strings.Contains(err.Error(), brand.EnvDatabaseURL) {
		t.Fatalf("hosted error should name %s, got %v", brand.EnvDatabaseURL, err)
	}
}

func TestReadFile(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()

	body, present, err := ReadFile(dir)
	if err != nil || present || body != "" {
		t.Fatalf("missing: body=%q present=%v err=%v", body, present, err)
	}

	path := filepath.Join(dir, brand.OfferingFile)
	if err := os.WriteFile(path, []byte("hosted\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	body, present, err = ReadFile(dir)
	if err != nil || !present || body != "hosted\n" {
		t.Fatalf("present: body=%q present=%v err=%v", body, present, err)
	}

	notAFile := t.TempDir()
	if err := os.Mkdir(filepath.Join(notAFile, brand.OfferingFile), 0o700); err != nil {
		t.Fatal(err)
	}
	if _, _, err := ReadFile(notAFile); err == nil {
		t.Fatal("offering is a directory: want error")
	}
}

func TestDir(t *testing.T) {
	t.Parallel()
	got, err := Dir("/var/lib/initagent")
	if err != nil || got != "/var/lib/initagent" {
		t.Fatalf("explicit: %q %v", got, err)
	}
	got, err = Dir("  ")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasSuffix(got, brand.ConfigDir) {
		t.Fatalf("default %q should end with %s", got, brand.ConfigDir)
	}
}
