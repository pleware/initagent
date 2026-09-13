package gateway

import (
	"context"
	"fmt"
	"net/http"
	"strings"

	"github.com/pleware/initagent/internal/connectorops"
	"github.com/pleware/initagent/internal/protocol"
)

// liveHello is liveConn plus the Hello handshake, which the setup probe
// needs for OS/arch. Both resolve through the same project-scoped socket
// so a device from another project answers offline here too (01).
func (g *Gateway) liveHello(w http.ResponseWriter, r *http.Request) (*agentConn, protocol.Hello, bool) {
	c := g.liveConn(w, r)
	if c == nil {
		return nil, protocol.Hello{}, false
	}
	p, ok := g.presence(r.PathValue("id"))
	if !ok {
		httpError(w, http.StatusServiceUnavailable, "connector is offline")
		return nil, protocol.Hello{}, false
	}
	return c, p.hello, true
}

func (g *Gateway) handleFsList(w http.ResponseWriter, r *http.Request) {
	c := g.liveConn(w, r)
	if c == nil {
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), sessionRPCTimeout)
	defer cancel()
	res, err := connectorops.ListDir(ctx, c, r.URL.Query().Get("path"))
	if err != nil {
		httpError(w, http.StatusBadGateway, err.Error())
		return
	}
	if res.Entries == nil {
		res.Entries = []protocol.FsEntry{}
	}
	writeJSON(w, res)
}

func (g *Gateway) handleFsDownload(w http.ResponseWriter, r *http.Request) {
	c := g.liveConn(w, r)
	if c == nil {
		return
	}
	path := r.URL.Query().Get("path")
	if path == "" {
		httpError(w, http.StatusBadRequest, "path required")
		return
	}
	base := path[strings.LastIndex(path, "/")+1:]
	w.Header().Set("Content-Disposition", fmt.Sprintf("attachment; filename=%q", base))
	w.Header().Set("Content-Type", "application/octet-stream")
	if err := connectorops.Download(c, path, w); err != nil {
		// A failure before the first byte is unreportable once the stream
		// headers are set; a dropped device surfaces as a truncated download.
		return
	}
}

func (g *Gateway) handleFsUpload(w http.ResponseWriter, r *http.Request) {
	c := g.liveConn(w, r)
	if c == nil {
		return
	}
	dir := r.URL.Query().Get("dir")
	if dir == "" {
		httpError(w, http.StatusBadRequest, "dir required")
		return
	}
	if err := r.ParseMultipartForm(32 << 20); err != nil {
		httpError(w, http.StatusBadRequest, "expected multipart upload")
		return
	}
	file, header, err := r.FormFile("file")
	if err != nil {
		httpError(w, http.StatusBadRequest, "file field required")
		return
	}
	defer file.Close()
	name := header.Filename
	if name == "" || strings.ContainsAny(name, "/\\") {
		httpError(w, http.StatusBadRequest, "bad filename")
		return
	}
	target := strings.TrimRight(dir, "/") + "/" + name
	if err := connectorops.Upload(c, target, file); err != nil {
		httpError(w, http.StatusBadGateway, err.Error())
		return
	}
	writeJSON(w, map[string]string{"path": target})
}

func (g *Gateway) handleSetupStatus(w http.ResponseWriter, r *http.Request) {
	c, hello, ok := g.liveHello(w, r)
	if !ok {
		return
	}
	writeJSON(w, connectorops.SetupStatus(r.Context(), c, hello.OS, hello.Arch))
}
