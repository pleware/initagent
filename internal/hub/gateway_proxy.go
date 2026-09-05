package hub

import (
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/pleware/initagent/internal/brand"
)

// gatewayProxyTimeout is the ceiling for fast gateway reads and mints that
// return immediately (device list, enroll token).
const gatewayProxyTimeout = 10 * time.Second

// proxyGateway forwards a request to the gateway named by p. The body streams
// through so POSTs (enroll tokens, task submissions) reach the gateway
// unchanged; timeout bounds the round trip.
//
// The project travels as a header rather than in the body, so a GET routes
// the same way a POST does and the body stays untouched. The secret is the
// hub proving it is the hub; it carries no project scope, which is why the
// gateway checks the header separately (09).
func (s *Server) proxyGateway(w http.ResponseWriter, r *http.Request, p placement, method, path string, timeout time.Duration) {
	u := strings.TrimRight(p.gatewayURL, "/") + path
	req, err := http.NewRequestWithContext(r.Context(), method, u, r.Body)
	if err != nil {
		httpError(w, http.StatusBadGateway, err.Error())
		return
	}
	if ct := r.Header.Get("Content-Type"); ct != "" {
		req.Header.Set("Content-Type", ct)
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
