package funnel

import "testing"

func TestResolveCTA(t *testing.T) {
	t.Parallel()
	cases := []struct {
		which, to, kind, loc string
		ok                   bool
	}{
		{"open_app", "", KindCTAOpenApp, OpenAppLocation, true},
		{"open_app", "https://evil.example/x", KindCTAOpenApp, OpenAppLocation, true},
		{"self_host", "", KindCTASelfHost, DefaultSelfHostURL, true},
		{"self_host", "https://initagent.dev/developers", KindCTASelfHost, "https://initagent.dev/developers", true},
		{"self_host", "https://www.initagent.dev/developers", KindCTASelfHost, "https://www.initagent.dev/developers", true},
		{"self_host", "http://localhost:5173/developers", KindCTASelfHost, "http://localhost:5173/developers", true},
		{"self_host", "http://127.0.0.1:5173/developers", KindCTASelfHost, "http://127.0.0.1:5173/developers", true},
		{"self_host", "https://evil.example/phish", KindCTASelfHost, DefaultSelfHostURL, true},
		{"self_host", "javascript:alert(1)", KindCTASelfHost, DefaultSelfHostURL, true},
		{"self_host", "//evil.example/x", KindCTASelfHost, DefaultSelfHostURL, true},
		{"self_host", "/developers", KindCTASelfHost, DefaultSelfHostURL, true},
		{"self_host", "ftp://initagent.dev/x", KindCTASelfHost, DefaultSelfHostURL, true},
		{"nope", "", "", "", false},
		{"", "", "", "", false},
	}
	for _, tc := range cases {
		kind, loc, ok := ResolveCTA(tc.which, tc.to)
		if ok != tc.ok || kind != tc.kind || loc != tc.loc {
			t.Errorf("ResolveCTA(%q, %q) = (%q, %q, %v), want (%q, %q, %v)",
				tc.which, tc.to, kind, loc, ok, tc.kind, tc.loc, tc.ok)
		}
	}
}
