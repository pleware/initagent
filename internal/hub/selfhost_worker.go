package hub

import (
	"cmp"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"path/filepath"
	"strings"

	"github.com/pleware/initagent/internal/agent"
	"github.com/pleware/initagent/internal/brand"
	"github.com/pleware/initagent/internal/offering"
)

func (s *Server) localConnectorPath() string {
	return filepath.Join(s.opts.DataDir, brand.ConnectorConfigFile)
}

// shouldBindSelfhostWorker is the first-project door: this box becomes a
// gateway worker only on self-host, only when a gateway exists, and only
// when the new row is the org's first project and has no machine yet.
func shouldBindSelfhostWorker(kind offering.Kind, gatewayURL string, project *Project, projectCount int) bool {
	if kind != offering.Selfhost {
		return false
	}
	if strings.TrimSpace(gatewayURL) == "" {
		return false
	}
	if project == nil || strings.TrimSpace(project.DeviceId) != "" {
		return false
	}
	return projectCount == 1
}

func (s *Server) bindSelfhostWorker(ctx context.Context, project *Project) *Project {
	if project == nil {
		return project
	}
	count, err := s.store.CountProjects(project.OrgId)
	if err != nil {
		log.Printf("self-host worker: count projects: %v", err)
		return project
	}
	gatewayURL := cmp.Or(strings.TrimSpace(project.GatewayURL), strings.TrimSpace(s.opts.GatewayURL))
	if !shouldBindSelfhostWorker(s.opts.Offering, gatewayURL, project, count) {
		return project
	}
	cfg, err := agent.LoadConfigFrom(s.localConnectorPath())
	if err != nil {
		token, mintErr := s.mintGatewayEnrollToken(ctx, gatewayURL, project.Id)
		if mintErr != nil {
			log.Printf("self-host worker: mint enroll token: %v", mintErr)
			return project
		}
		cfg, err = agent.EnrollTo(gatewayURL, token, s.localConnectorPath())
		if err != nil {
			log.Printf("self-host worker: enroll: %v", err)
			return project
		}
	}
	if s.signalSelfhostWorker != nil {
		s.signalSelfhostWorker()
	}
	updated, err := s.store.UpdateProject(project.Id, project.Name, cfg.DeviceId, project.Path, project.TemplateId, project.RepoRemote, project.RepoHost)
	if err != nil {
		log.Printf("self-host worker: attach device: %v", err)
		return project
	}
	if updated == nil {
		return project
	}
	log.Printf("self-host worker enrolled as %s on %s", cfg.DeviceId, project.Id)
	return updated
}

func (s *Server) mintGatewayEnrollToken(ctx context.Context, gatewayURL, projectID string) (string, error) {
	u := strings.TrimRight(gatewayURL, "/") + "/api/enroll-tokens"
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, u, nil)
	if err != nil {
		return "", err
	}
	req.Header.Set(brand.ProjectHeader, projectID)
	if s.opts.GatewaySecret != "" {
		req.Header.Set("Authorization", "Bearer "+s.opts.GatewaySecret)
	}
	resp, err := (&http.Client{Timeout: gatewayProxyTimeout}).Do(req)
	if err != nil {
		return "", fmt.Errorf("reaching gateway: %w", err)
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(io.LimitReader(resp.Body, 64*1024))
	if err != nil {
		return "", err
	}
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("gateway refused enroll token (%s): %s", resp.Status, strings.TrimSpace(string(body)))
	}
	var offer struct {
		Token string `json:"token"`
	}
	if err := json.Unmarshal(body, &offer); err != nil {
		return "", fmt.Errorf("parsing enroll token: %w", err)
	}
	if offer.Token == "" {
		return "", fmt.Errorf("gateway returned an empty enroll token")
	}
	return offer.Token, nil
}

func (s *Server) runSelfhostGatewayAgent(ctx context.Context) {
	path := s.localConnectorPath()
	if _, err := agent.LoadConfigFrom(path); err != nil {
		select {
		case <-s.selfhostWorkerReady:
		case <-ctx.Done():
			return
		}
	}
	cfg, err := agent.LoadConfigFrom(path)
	if err != nil {
		log.Printf("self-host worker: %v", err)
		return
	}
	if err := agent.New(cfg, s.opts.Version).Run(ctx); err != nil && ctx.Err() == nil {
		log.Printf("embedded agent stopped: %v", err)
	}
}

// recoverSelfhostWorker binds this box onto a first project that was created
// before a gateway URL existed — local `serve` without --gateway-url, then a
// restart that starts the companion. Create already calls bindSelfhostWorker.
func (s *Server) recoverSelfhostWorker(ctx context.Context) {
	projects, err := s.store.ListAllProjects()
	if err != nil {
		log.Printf("self-host worker: list projects: %v", err)
		return
	}
	if len(projects) != 1 {
		return
	}
	s.bindSelfhostWorker(ctx, &projects[0])
}
