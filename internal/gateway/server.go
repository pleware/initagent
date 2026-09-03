package gateway

import (
	"context"
	"encoding/json"
	"net"
	"net/http"
	"time"
)

// Health is the JSON body of GET /health.
type Health struct {
	OK        bool   `json:"ok"`
	ProjectID string `json:"project_id"`
	Addr      string `json:"addr"`
}

// Handler serves the Milestone 0 process surface: health only.
func (g *Gateway) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /health", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(Health{
			OK:        true,
			ProjectID: g.project.ID,
			Addr:      g.addr,
		})
	})
	return mux
}

// Serve listens on addr until ctx is cancelled.
func (g *Gateway) Serve(ctx context.Context, addr string) error {
	if addr == "" {
		addr = g.addr
	}
	ln, err := net.Listen("tcp", addr)
	if err != nil {
		return err
	}
	g.addr = ln.Addr().String()

	srv := &http.Server{Handler: g.Handler()}
	errc := make(chan error, 1)
	go func() {
		errc <- srv.Serve(ln)
	}()

	select {
	case <-ctx.Done():
		shut, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		defer cancel()
		_ = srv.Shutdown(shut)
		return ctx.Err()
	case err := <-errc:
		if err == http.ErrServerClosed {
			return nil
		}
		return err
	}
}
