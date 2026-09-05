package repo

import "testing"

func TestHostFromRemote(t *testing.T) {
	t.Parallel()
	cases := []struct {
		in      string
		want    Host
		wantErr bool
	}{
		{"", "", true},
		{"   ", "", true},
		{"https://github.com/acme/app.git", GitHub, false},
		{"git@github.com:acme/app.git", GitHub, false},
		{"ssh://git@github.com/acme/app.git", GitHub, false},
		{"https://gitlab.com/acme/app", GitLab, false},
		{"https://gitlab.example.com/acme/app", GitLab, false},
		{"git@bitbucket.org:acme/app.git", Bitbucket, false},
		{"https://bitbucket.org/acme/app", Bitbucket, false},
		{"https://codeberg.org/acme/app", Gitea, false},
		{"https://gitea.example.com/acme/app", Gitea, false},
		{"https://forgejo.example.com/acme/app", Forgejo, false},
		{"https://git.acme.internal/app.git", PlainGit, false},
		{"/local/path", PlainGit, false},
	}
	for _, tc := range cases {
		t.Run(tc.in, func(t *testing.T) {
			t.Parallel()
			got, err := HostFromRemote(tc.in)
			if tc.wantErr {
				if err == nil {
					t.Fatalf("HostFromRemote(%q) = %q, want error", tc.in, got)
				}
				return
			}
			if err != nil {
				t.Fatalf("HostFromRemote(%q): %v", tc.in, err)
			}
			if got != tc.want {
				t.Fatalf("HostFromRemote(%q) = %q, want %q", tc.in, got, tc.want)
			}
		})
	}
}
