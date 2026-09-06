package hub

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"net/netip"
	"testing"

	"github.com/pleware/initagent/internal/offering"
)

func TestParseTrustedProxies(t *testing.T) {
	got, err := parseTrustedProxies(" 172.16.0.0/12, 10.0.0.1, 2001:db8::/32 ")
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 3 {
		t.Fatalf("len = %d, want 3", len(got))
	}
	if !got[0].Contains(netip.MustParseAddr("172.18.0.4")) {
		t.Fatalf("docker CIDR missed 172.18.0.4: %v", got[0])
	}
	if got[1].Bits() != 32 {
		t.Fatalf("single IPv4 bits = %d, want 32", got[1].Bits())
	}
}

func TestParseTrustedProxiesEmpty(t *testing.T) {
	got, err := parseTrustedProxies("  ")
	if err != nil || got != nil {
		t.Fatalf("got %#v %v", got, err)
	}
}

func TestParseTrustedProxiesRejectsJunk(t *testing.T) {
	if _, err := parseTrustedProxies("not-an-addr"); err == nil {
		t.Fatal("want error")
	}
}

func TestNewServerRejectsBadTrustedProxies(t *testing.T) {
	_, err := NewServer(Options{
		Addr:           "127.0.0.1:0",
		DataDir:        t.TempDir(),
		Offering:       offering.Selfhost,
		TrustedProxies: "not-a-cidr",
	})
	if err == nil {
		t.Fatal("want error")
	}
}

func TestClientKeyIgnoresForwardedWhenPeerIsUntrusted(t *testing.T) {
	trusted, err := parseTrustedProxies("172.16.0.0/12")
	if err != nil {
		t.Fatal(err)
	}
	got := clientKey("203.0.113.9:443", "8.8.8.8", trusted)
	if got != "203.0.113.9" {
		t.Fatalf("got %q, want the TCP peer", got)
	}
}

func TestClientKeyTakesRightmostUntrustedHop(t *testing.T) {
	trusted, err := parseTrustedProxies("172.16.0.0/12")
	if err != nil {
		t.Fatal(err)
	}
	// Spoofed leftmost hop must not win: Caddy appends the real client.
	got := clientKey("172.18.0.2:1234", "8.8.8.8, 203.0.113.50", trusted)
	if got != "203.0.113.50" {
		t.Fatalf("got %q, want the real client", got)
	}
}

func TestClientKeyEmptyTrustedIgnoresHeader(t *testing.T) {
	got := clientKey("203.0.113.9:443", "8.8.8.8", nil)
	if got != "203.0.113.9" {
		t.Fatalf("got %q", got)
	}
}

func TestClientKeyAllTrustedFallsBackToPeer(t *testing.T) {
	trusted, err := parseTrustedProxies("172.16.0.0/12")
	if err != nil {
		t.Fatal(err)
	}
	got := clientKey("172.18.0.2:1234", "172.18.0.4, 172.18.0.5", trusted)
	if got != "172.18.0.2" {
		t.Fatalf("got %q, want the TCP peer", got)
	}
}

func TestClientKeySkipsJunkHops(t *testing.T) {
	trusted, err := parseTrustedProxies("172.16.0.0/12")
	if err != nil {
		t.Fatal(err)
	}
	got := clientKey("172.18.0.2:1234", "not-an-ip, 203.0.113.50", trusted)
	if got != "203.0.113.50" {
		t.Fatalf("got %q", got)
	}
}

func TestClientKeyUnmapsIPv4Mapped(t *testing.T) {
	trusted, err := parseTrustedProxies("172.16.0.0/12")
	if err != nil {
		t.Fatal(err)
	}
	got := clientKey("[::ffff:172.18.0.2]:1234", "203.0.113.50", trusted)
	if got != "203.0.113.50" {
		t.Fatalf("got %q", got)
	}
}

func TestRateLimitBucketsByForwardedClient(t *testing.T) {
	srv := newHub(t, t.TempDir(), offering.Selfhost)
	var err error
	srv.trusted, err = parseTrustedProxies("127.0.0.0/8")
	if err != nil {
		t.Fatal(err)
	}
	ts := httptest.NewServer(srv.mux)
	t.Cleanup(ts.Close)

	hit := func(xff string) int {
		t.Helper()
		b, err := json.Marshal(map[string]string{
			"email": "a@example.com", "password": "wrong-password-ok",
		})
		if err != nil {
			t.Fatal(err)
		}
		req, err := http.NewRequest(http.MethodPost, ts.URL+"/api/login", bytes.NewReader(b))
		if err != nil {
			t.Fatal(err)
		}
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("X-Forwarded-For", xff)
		resp, err := ts.Client().Do(req)
		if err != nil {
			t.Fatal(err)
		}
		defer resp.Body.Close()
		io.Copy(io.Discard, resp.Body)
		return resp.StatusCode
	}

	for i := range rateMax {
		if code := hit("203.0.113.10"); code == http.StatusTooManyRequests {
			t.Fatalf("client A blocked at attempt %d", i+1)
		}
	}
	if code := hit("203.0.113.10"); code != http.StatusTooManyRequests {
		t.Fatalf("client A after %d = %d, want 429", rateMax, code)
	}
	if code := hit("203.0.113.11"); code == http.StatusTooManyRequests {
		t.Fatalf("client B shared A's bucket")
	}
}
