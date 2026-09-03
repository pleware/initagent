package hub

import (
	"io"
	"net/http"
	"strings"
	"time"
)

// gatewayProxyTimeout is the ceiling for fast gateway reads and mints that
// return immediately (device list, enroll token).
const gatewayProxyTimeout = 10 * time.Second

// proxyGateway forwards a request to the project gateway. The body streams
// through so POSTs (enroll tokens, task submissions) reach the gateway
// unchanged; timeout bounds the round trip.
func (s *Server) proxyGateway(w http.ResponseWriter, r *http.Request, method, path string, timeout time.Duration) {
	u := strings.TrimRight(s.opts.GatewayURL, "/") + path
	req, err := http.NewRequestWithContext(r.Context(), method, u, r.Body)
	if err != nil {
		httpError(w, http.StatusBadGateway, err.Error())
		return
	}
	if ct := r.Header.Get("Content-Type"); ct != "" {
		req.Header.Set("Content-Type", ct)
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
