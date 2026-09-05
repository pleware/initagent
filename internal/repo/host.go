// Package repo classifies a clone URL into the closed host enumeration
// from draft 14. It does not open a PR, store credentials, or mint a
// rep- row — boarding only needs to know which host the remote is.
package repo

import (
	"fmt"
	"net/url"
	"strings"
)

// Host is a git host adapter key. The set is closed and lives in code,
// not as database rows.
type Host string

const (
	GitHub    Host = "github"
	GitLab    Host = "gitlab"
	Bitbucket Host = "bitbucket"
	Gitea     Host = "gitea"
	Forgejo   Host = "forgejo"
	PlainGit  Host = "plain-git"
)

// HostFromRemote classifies a clone URL. An empty string is an error; every
// other non-empty value maps to a host, falling back to plain-git.
func HostFromRemote(remote string) (Host, error) {
	s := strings.TrimSpace(remote)
	if s == "" {
		return "", fmt.Errorf("repo: empty remote")
	}
	return classify(hostname(s)), nil
}

func hostname(remote string) string {
	if host, _, ok := strings.Cut(remote, ":"); ok && !strings.Contains(host, "://") {
		// scp-like: git@github.com:owner/repo.git
		if _, after, found := strings.Cut(host, "@"); found {
			return strings.ToLower(after)
		}
	}
	if strings.Contains(remote, "://") {
		u, err := url.Parse(remote)
		if err == nil && u.Host != "" {
			return strings.ToLower(u.Hostname())
		}
	}
	return strings.ToLower(remote)
}

func classify(host string) Host {
	host = strings.TrimPrefix(host, "www.")
	switch {
	case host == "github.com" || strings.HasSuffix(host, ".github.com"):
		return GitHub
	case host == "gitlab.com" || strings.Contains(host, "gitlab."):
		return GitLab
	case host == "bitbucket.org" || strings.HasSuffix(host, ".bitbucket.org"):
		return Bitbucket
	case host == "codeberg.org" || strings.Contains(host, "gitea."):
		return Gitea
	case strings.Contains(host, "forgejo"):
		return Forgejo
	default:
		return PlainGit
	}
}
