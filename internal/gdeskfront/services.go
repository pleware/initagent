package gdeskfront

import (
	"fmt"
	"net"
	"net/url"
	"strconv"

	"github.com/pleware/initagent/internal/gdesk"
	"github.com/pleware/initagent/internal/gdeskconsole"
	"github.com/pleware/initagent/internal/gdeskseam"
)

// sensingProducer is the sibling that turns a camera into facts on this box.
//
// The contract is `os:desk-facts` in the PWare OS registry: that process writes
// one JSON fact per line on stdout — attendance and range, never a frame and
// never an identity — and this connector is the reader, as the parent that
// spawns it.
//
// sensingProducer is the sibling repository that implements pware-vision.
const sensingProducer = "pware-os-input-vision"

// Services is what this box runs, for the operator console's list.
//
// Assembled here because this is the only package that knows all of it: the
// address came from the seam configuration, the routes are this file's own mux,
// the roles are the provider inventory, and the client count belongs to the
// listener. Every row is either evidence — a port this process holds, a count
// taken now — or carries ServiceDeclared, and nothing in between.
func (d *Desk) Services() []gdeskseam.Service {
	if d.listener == nil {
		return nil
	}
	addr := d.Addr()
	// One port for all three rows, and it is the one the listener took rather
	// than the one configuration asked for: a desk started on `:0` holds a port
	// nobody wrote down, and that is exactly the case where somebody is looking.
	local := portOf(addr)
	clients := d.socket.Clients()
	out := []gdeskseam.Service{{
		Name:    "gdesk seam",
		Where:   addr + gdeskseam.Path,
		Port:    local,
		State:   gdeskseam.ServiceListening,
		Clients: &clients,
		Note:    "websocket; one stream per connected client",
	}, {
		Name:  "operator console",
		Where: addr + gdeskconsole.Path,
		Port:  local,
		State: gdeskseam.ServiceListening,
		// No count on purpose: an HTTP route holds nobody between requests, so
		// a number here would be a guess dressed as a measurement. Whoever has
		// this page open is on the seam, and shows up on the row above.
		Note: "this page; its own seam connection is counted above",
	}, {
		Name:  "operator log",
		Where: addr + gdeskseam.LogsPath,
		Port:  local,
		State: gdeskseam.ServiceListening,
		Note:  "polled by the console once a second",
	}}

	out = append(out, d.roleServices()...)

	return append(out, d.sensingService())
}

func (d *Desk) sensingService() gdeskseam.Service {
	row := gdeskseam.Service{
		Name:  "local sensing",
		Where: "stdout, no port",
	}
	if d.sensors == nil {
		row.State = gdeskseam.ServiceDeclared
		row.Note = sensingProducer + " writes NDJSON facts; set vision.camera to start one"
		return row
	}
	row.State = gdeskseam.ServiceListening
	row.Note = sensingNote(d.sensors.State(), sensingProducer)
	return row
}

// roleServices is one row per desk role: where its provider is, or why it is
// silent. The port in a provider row is somebody else's, which is what
// ServiceOutbound says and why no row here carries a client count.
func (d *Desk) roleServices() []gdeskseam.Service {
	out := make([]gdeskseam.Service, 0, len(gdesk.Roles))
	for _, role := range gdesk.Roles {
		binding, bound := d.config.Binding(role)
		if !bound {
			out = append(out, gdeskseam.Service{
				Name:  string(role),
				Where: "nothing bound",
				State: gdeskseam.ServiceSilent,
				Note:  silenceNote(d.config, role),
			})
			continue
		}
		// Address and model, never the key: a provider entry is the half of the
		// configuration that can be pasted into a bug report (gdesk.Provider).
		out = append(out, gdeskseam.Service{
			Name:  string(role),
			Where: binding.Provider.BaseURL,
			Port:  providerPort(binding.Provider.BaseURL),
			State: gdeskseam.ServiceOutbound,
			Note:  fmt.Sprintf("%s via %s", binding.Model, binding.Provider.ID),
		})
	}
	return out
}

// portOf is the port a bound local address holds. An address that is not
// host:port by the time it is reported is not something to guess about, so the
// row simply carries no port.
func portOf(addr string) int {
	_, port, err := net.SplitHostPort(addr)
	if err != nil {
		return 0
	}
	number, err := strconv.Atoi(port)
	if err != nil {
		return 0
	}
	return number
}

// providerPort is the port a base URL reaches. Most provider URLs carry no port
// and still reach 443, so the scheme's own default is the truthful answer — an
// empty cell there would suggest the desk calls nowhere. Anything else, a local
// model on a chosen port included, is read from the URL.
func providerPort(base string) int {
	parsed, err := url.Parse(base)
	if err != nil || parsed.Host == "" {
		return 0
	}
	if port := parsed.Port(); port != "" {
		number, err := strconv.Atoi(port)
		if err != nil {
			return 0
		}
		return number
	}
	switch parsed.Scheme {
	case "https":
		return 443
	case "http":
		return 80
	}
	return 0
}

// silenceNote is why a role cannot answer, in the words the desk already prints
// at startup. Unbound and "the key has not arrived yet" are different problems
// and cost different afternoons.
func silenceNote(cfg gdesk.Config, role gdesk.Role) string {
	for _, silence := range cfg.Silences() {
		if silence.Role != role {
			continue
		}
		if silence.Provider == "" {
			return string(silence.Reason)
		}
		return fmt.Sprintf("%s (%s)", silence.Reason, silence.Provider)
	}
	return ""
}
