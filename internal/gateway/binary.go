package gateway

import (
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/pleware/initagent/internal/brand"
)

func (g *Gateway) handleAgentBinary(w http.ResponseWriter, r *http.Request) {
	osName := r.URL.Query().Get("os")
	arch := r.URL.Query().Get("arch")
	if osName == "" || arch == "" {
		httpError(w, http.StatusBadRequest, "os and arch required")
		return
	}
	if !isSimpleToken(osName) || !isSimpleToken(arch) {
		httpError(w, http.StatusBadRequest, "invalid os or arch")
		return
	}
	if osName == runtime.GOOS && arch == runtime.GOARCH {
		exe, err := os.Executable()
		if err == nil {
			if resolved, rerr := filepath.EvalSymlinks(exe); rerr == nil {
				exe = resolved
			}
			w.Header().Set("Content-Type", "application/octet-stream")
			http.ServeFile(w, r, exe)
			return
		}
	}
	candidate := filepath.Join(g.dataDir, "binaries", brand.ReleaseAsset(osName, arch))
	if _, err := os.Stat(candidate); err == nil {
		w.Header().Set("Content-Type", "application/octet-stream")
		http.ServeFile(w, r, candidate)
		return
	}
	if g.githubRepo != "" {
		asset := brand.ReleaseAsset(osName, arch)
		tag := "latest/download"
		if v := g.version; v != "" && strings.HasPrefix(v, "v") && !strings.Contains(v, "-") {
			tag = "download/" + v
		}
		url := fmt.Sprintf("https://github.com/%s/releases/%s/%s", g.githubRepo, tag, asset)
		http.Redirect(w, r, url, http.StatusFound)
		return
	}
	httpError(w, http.StatusNotFound, fmt.Sprintf("no binary for %s/%s", osName, arch))
}
