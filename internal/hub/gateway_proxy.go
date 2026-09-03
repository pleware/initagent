package hub

import (
	"io"
	"net/http"
	"strings"
	"time"
)

func (s *Server) proxyGateway(w http.ResponseWriter, r *http.Request, method, path string) {
	u := strings.TrimRight(s.opts.GatewayURL, "/") + path
	req, err := http.NewRequestWithContext(r.Context(), method, u, nil)
	if err != nil {
		httpError(w, http.StatusBadGateway, err.Error())
		return
	}
	client := &http.Client{Timeout: 10 * time.Second}
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
