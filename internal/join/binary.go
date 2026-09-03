package join

import (
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/pleware/initagent/internal/brand"
)

// ServeBinary handles GET /api/agent-binary?os=&arch=, the download the join
// script performs before it enrolls.
//
// Resolution order: this process's own executable when the platform matches,
// then a binary dropped into DataDir/binaries, then a redirect to the matching
// GitHub release asset. The first two make a same-LAN join work with no
// internet at all; the third keeps a cross-platform join working without the
// plane carrying every binary.
func (i Installer) ServeBinary(w http.ResponseWriter, r *http.Request) {
	osName := r.URL.Query().Get("os")
	arch := r.URL.Query().Get("arch")
	if osName == "" || arch == "" {
		httpError(w, http.StatusBadRequest, "os and arch required")
		return
	}
	// os/arch become part of a filesystem path and a redirect URL.
	if !SafeToken(osName) || !SafeToken(arch) {
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
	candidate := filepath.Join(i.DataDir, "binaries", brand.ReleaseAsset(osName, arch))
	if _, err := os.Stat(candidate); err == nil {
		w.Header().Set("Content-Type", "application/octet-stream")
		http.ServeFile(w, r, candidate)
		return
	}
	if i.GithubRepo != "" {
		asset := brand.ReleaseAsset(osName, arch)
		// Prefer the release matching this process; a dev build has no release.
		tag := "latest/download"
		if v := i.Version; v != "" && strings.HasPrefix(v, "v") && !strings.Contains(v, "-") {
			tag = "download/" + v
		}
		url := fmt.Sprintf("https://github.com/%s/releases/%s/%s", i.GithubRepo, tag, asset)
		http.Redirect(w, r, url, http.StatusFound)
		return
	}
	httpError(w, http.StatusNotFound, fmt.Sprintf(
		"no binary for %s/%s — cross-compile with `GOOS=%s GOARCH=%s go build -o %s ./cmd/initagent`, then retry",
		osName, arch, osName, arch, candidate))
}
