package hub

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/pleware/initagent/internal/brand"
)

// gatewayProxyTimeout is the ceiling for fast gateway reads and mints that
// return immediately (connector list, enroll token).
const gatewayProxyTimeout = 10 * time.Second

// connectorProxyTimeout covers session create/list/kill and exec round-trips
// (the connector's own timeout plus slack).
const connectorProxyTimeout = 90 * time.Second

// proxyGateway forwards a request to the gateway named by p. The body streams
// through so POSTs (enroll tokens, task submissions) reach the gateway
// unchanged; timeout bounds the round trip.
//
// The project travels as a header rather than in the body, so a GET routes
// the same way a POST does and the body stays untouched. The secret is the
// hub proving it is the hub; it carries no project scope, which is why the
// gateway checks the header separately (09).
func (s *Server) proxyGateway(w http.ResponseWriter, r *http.Request, p placement, method, path string, timeout time.Duration) {
	s.proxyGatewayBody(w, r, p, method, path, timeout, r.Body)
}

// proxyGatewayJSON forwards a marshalled body to the gateway named by p. It
// is the body-aware form of proxyGateway for a hop whose outbound payload is
// not the request's own body: project exec owns cwd and the timeout unit and
// rewrites them before the hop, so the request body cannot stream through
// unchanged.
func (s *Server) proxyGatewayJSON(w http.ResponseWriter, r *http.Request, p placement, method, path string, timeout time.Duration, body any) {
	payload, err := json.Marshal(body)
	if err != nil {
		httpError(w, http.StatusInternalServerError, err.Error())
		return
	}
	s.proxyGatewayBody(w, r, p, method, path, timeout, bytes.NewReader(payload))
}

// proxyGatewayBody is the single forwarding path shared by proxyGateway and
// proxyGatewayJSON. It builds the request, carries the project as a header
// and the secret as the hub's proof, then streams the reply back bounded by
// timeout.
func (s *Server) proxyGatewayBody(w http.ResponseWriter, r *http.Request, p placement, method, path string, timeout time.Duration, body io.Reader) {
	u := strings.TrimRight(p.gatewayURL, "/") + path
	req, err := http.NewRequestWithContext(r.Context(), method, u, body)
	if err != nil {
		httpError(w, http.StatusBadGateway, err.Error())
		return
	}
	if ct := r.Header.Get("Content-Type"); ct != "" {
		req.Header.Set("Content-Type", ct)
	} else {
		req.Header.Set("Content-Type", "application/json")
	}
	if p.projectID != "" {
		req.Header.Set(brand.ProjectHeader, p.projectID)
	}
	if s.opts.GatewaySecret != "" {
		req.Header.Set("Authorization", "Bearer "+s.opts.GatewaySecret)
	}
	client := &http.Client{Timeout: timeout}
	resp, err := client.Do(req)
	if err != nil {
		httpError(w, http.StatusBadGateway, "gateway unreachable: "+err.Error())
		return
	}
	defer resp.Body.Close()
	for _, k := range []string{"Content-Type"} {
		if v := resp.Header.Get(k); v != "" {
			w.Header().Set(k, v)
		}
	}
	w.WriteHeader(resp.StatusCode)
	_, _ = io.Copy(w, io.LimitReader(resp.Body, 1<<20))
}

// getGatewayJSON performs an authenticated GET against the gateway named by
// p and decodes the JSON reply into out. It is the fan-out shape the proxy
// does not cover: fleet-wide views ask several gateways at once and merge the
// answers, so they need the decoded body rather than a streamed copy.
func (s *Server) getGatewayJSON(ctx context.Context, p placement, path string, out any) error {
	u := strings.TrimRight(p.gatewayURL, "/") + path
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return err
	}
	if p.projectID != "" {
		req.Header.Set(brand.ProjectHeader, p.projectID)
	}
	if s.opts.GatewaySecret != "" {
		req.Header.Set("Authorization", "Bearer "+s.opts.GatewaySecret)
	}
	client := &http.Client{Timeout: gatewayProxyTimeout}
	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		_, _ = io.Copy(io.Discard, resp.Body)
		return fmt.Errorf("gateway %s answered %s", p.gatewayURL, resp.Status)
	}
	return json.NewDecoder(io.LimitReader(resp.Body, 1<<20)).Decode(out)
}

// proxyGatewayStream forwards a request whose reply is an unbounded stream
// (file download): no 1 MiB cap and no timeout, so a large file transfers at
// its own pace and is only bounded by the caller's request context. It also
// forwards Content-Disposition so the browser still gets a filename.
func (s *Server) proxyGatewayStream(w http.ResponseWriter, r *http.Request, p placement, method, path string) {
	u := strings.TrimRight(p.gatewayURL, "/") + path
	req, err := http.NewRequestWithContext(r.Context(), method, u, r.Body)
	if err != nil {
		httpError(w, http.StatusBadGateway, err.Error())
		return
	}
	if p.projectID != "" {
		req.Header.Set(brand.ProjectHeader, p.projectID)
	}
	if s.opts.GatewaySecret != "" {
		req.Header.Set("Authorization", "Bearer "+s.opts.GatewaySecret)
	}
	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		httpError(w, http.StatusBadGateway, "gateway unreachable: "+err.Error())
		return
	}
	defer resp.Body.Close()
	for _, k := range []string{"Content-Type", "Content-Disposition"} {
		if v := resp.Header.Get(k); v != "" {
			w.Header().Set(k, v)
		}
	}
	w.WriteHeader(resp.StatusCode)
	_, _ = io.Copy(w, resp.Body)
}
