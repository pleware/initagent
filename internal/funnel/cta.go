package funnel

import (
	"net"
	"net/url"
	"strings"
)

// CTA destinations the marketing site may ask us to bounce to.
const (
	WhichOpenApp       = "open_app"
	WhichSelfHost      = "self_host"
	OpenAppLocation    = "/"
	DefaultSelfHostURL = "https://initagent.dev/developers"
)

// ResolveCTA maps a public /r/cta/{which} hit onto a kind and a 302
// Location. which must be open_app or self_host. to is only used for
// self-host and must be an http(s) URL on an allowlisted host; anything
// else falls back to the public developers page so an open redirect
// cannot leave this origin.
func ResolveCTA(which, to string) (kind, location string, ok bool) {
	switch strings.TrimSpace(which) {
	case WhichOpenApp:
		return KindCTAOpenApp, OpenAppLocation, true
	case WhichSelfHost:
		return KindCTASelfHost, safeSelfHost(to), true
	default:
		return "", "", false
	}
}

func safeSelfHost(to string) string {
	raw := strings.TrimSpace(to)
	if raw == "" {
		return DefaultSelfHostURL
	}
	u, err := url.Parse(raw)
	if err != nil || u.Host == "" {
		return DefaultSelfHostURL
	}
	if u.Scheme != "http" && u.Scheme != "https" {
		return DefaultSelfHostURL
	}
	if !allowedCTAHost(u.Host) {
		return DefaultSelfHostURL
	}
	return u.String()
}

func allowedCTAHost(host string) bool {
	h := strings.ToLower(strings.TrimSpace(host))
	if name, _, err := net.SplitHostPort(h); err == nil {
		h = name
	}
	switch h {
	case "initagent.dev", "www.initagent.dev", "localhost", "127.0.0.1", "::1":
		return true
	default:
		return false
	}
}
