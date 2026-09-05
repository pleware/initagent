package agent

import (
	"context"
	"log"
	"os"
	"runtime"
	"time"

	"github.com/pleware/initagent/internal/brand"
	"github.com/pleware/initagent/internal/updater"
)

// managedEnv is set by the installed service definition. Foreground debug
// agents never replace themselves.
const managedEnv = brand.EnvManaged

// maybeSelfUpdate installs the exact stable release advertised by the hub.
// It never downgrades and every download is checksum + version verified by the
// shared updater package before the current executable is touched.
func (a *Agent) maybeSelfUpdate(ctx context.Context, hubVersion, repo string) bool {
	if os.Getenv(managedEnv) == "" || repo == "" || !updater.IsNewer(hubVersion, a.version) {
		return false
	}

	a.mu.Lock()
	if a.execsInFlight > 0 {
		// Swapping now would drop the reply the gateway is waiting on and
		// requeue the customer's run. Staying on the old build costs nothing
		// in comparison, so retry on a later tick instead of spending the
		// hourly budget below.
		a.mu.Unlock()
		return false
	}
	if time.Now().Before(a.nextUpdateAttempt) {
		a.mu.Unlock()
		return false
	}
	// A failure can be retried without reconnecting, but avoid hammering GitHub
	// or flapping a device when a release asset is temporarily unavailable.
	a.nextUpdateAttempt = time.Now().Add(time.Hour)
	a.mu.Unlock()

	release, err := updater.ForVersion(ctx, repo, hubVersion, runtime.GOOS, runtime.GOARCH)
	if err == nil {
		err = updater.Install(ctx, release, os.Getenv(brand.EnvWindowsTask))
	}
	if err != nil {
		log.Printf("self-update to %s failed (continuing on %s): %v", hubVersion, a.version, err)
		return false
	}
	log.Printf("self-update staged %s -> %s; restarting", a.version, hubVersion)
	return true
}

// idleRestartPoll is how often a staged binary re-checks for a free slot.
var idleRestartPoll = 5 * time.Second

func (a *Agent) execStarted() {
	a.mu.Lock()
	a.execsInFlight++
	a.mu.Unlock()
}

func (a *Agent) execDone() {
	a.mu.Lock()
	a.execsInFlight--
	a.mu.Unlock()
}

// restartWhenIdle drops the connection once no supervised command is running,
// so the service manager restarts into the staged binary between runs rather
// than in the middle of one. The swap already happened on disk; this process
// keeps serving the old build until a slot frees, which is the cheaper of the
// two failures. A command that starts during the download extends the wait.
func (a *Agent) restartWhenIdle(ctx context.Context) {
	ticker := time.NewTicker(idleRestartPoll)
	defer ticker.Stop()
	for {
		a.mu.Lock()
		idle := a.execsInFlight == 0
		conn := a.conn
		if idle {
			a.updateApplied = true
		}
		a.mu.Unlock()
		if idle {
			if conn != nil {
				conn.Close()
			}
			return
		}
		select {
		case <-ticker.C:
		case <-ctx.Done():
			return
		}
	}
}

// autoUpdateLoop retries a failed fleet update while a connection remains
// healthy. A successful swap closes the socket once the device is idle;
// runOnce observes the flag and exits so the service manager can restart the
// verified binary.
func (a *Agent) autoUpdateLoop(ctx context.Context, hubVersion, repo string) {
	ticker := time.NewTicker(15 * time.Minute)
	defer ticker.Stop()
	for {
		select {
		case <-ticker.C:
			if a.maybeSelfUpdate(ctx, hubVersion, repo) {
				a.restartWhenIdle(ctx)
				return
			}
		case <-ctx.Done():
			return
		}
	}
}
