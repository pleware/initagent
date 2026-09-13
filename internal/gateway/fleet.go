package gateway

import (
	"context"
	"net/http"
	"sort"
	"sync"

	"github.com/pleware/initagent/internal/protocol"
)

// fleetAgent is one session row on the project plane. The JSON shape matches
// the hub's fleetSession so the hub can decode and merge the reply without a
// second wire type (16).
type fleetAgent struct {
	protocol.Session
	ConnectorId   string `json:"connectorId"`
	ConnectorName string `json:"connectorName"`
}

// handleFleetAgents lists every session across the project's online connectors.
// The hub calls it once per reachable gateway and merges the answer with its
// own registry, so a worker that dialed this gateway still shows up on the
// fleet view (16).
func (g *Gateway) handleFleetAgents(w http.ResponseWriter, r *http.Request) {
	projectID, ok := g.resolveProject(w, r)
	if !ok {
		return
	}
	connectors, err := g.store.ListConnectors(r.Context(), projectID)
	if err != nil {
		httpError(w, http.StatusInternalServerError, err.Error())
		return
	}
	nameByID := make(map[string]string, len(connectors))
	for _, d := range connectors {
		nameByID[d.ID] = d.Name
	}

	// Collect the live sockets first, then release the lock, so the fan-out
	// does not hold the connection map while waiting on each connector's reply.
	var ids []string
	g.mu.Lock()
	for id, p := range g.online {
		if p.projectID == projectID && p.conn != nil {
			ids = append(ids, id)
		}
	}
	g.mu.Unlock()

	var mu sync.Mutex
	out := make([]fleetAgent, 0)
	var wg sync.WaitGroup
	for _, id := range ids {
		wg.Add(1)
		go func(connectorID string) {
			defer wg.Done()
			c := g.connForProject(projectID, connectorID)
			if c == nil {
				return
			}
			ctx, cancel := context.WithTimeout(r.Context(), sessionRPCTimeout)
			defer cancel()
			var res protocol.SessionsListResult
			if err := c.callInto(ctx, protocol.TypeSessionsList, nil, &res); err != nil {
				return
			}
			mu.Lock()
			for _, sess := range res.Sessions {
				out = append(out, fleetAgent{Session: sess, ConnectorId: connectorID, ConnectorName: nameByID[connectorID]})
			}
			mu.Unlock()
		}(id)
	}
	wg.Wait()
	sort.Slice(out, func(i, j int) bool {
		if out[i].ConnectorName != out[j].ConnectorName {
			return out[i].ConnectorName < out[j].ConnectorName
		}
		return out[i].Name < out[j].Name
	})
	writeJSON(w, out)
}
