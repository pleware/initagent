package main

import (
	"context"
	"flag"
	"fmt"
	"os"

	"github.com/pleware/initagent/internal/brand"
	"github.com/pleware/initagent/internal/gateway"
)

func cmdGateway(args []string) error {
	fs := flag.NewFlagSet("gateway", flag.ExitOnError)
	addr := fs.String("addr", ":4201", "listen address")
	dataDir := fs.String("data-dir", "", "data directory (default ~/.initagent)")
	projectID := fs.String("project", "", "shared prj- (minted on first start if empty)")
	publicURL := fs.String("public-url", "", "URL baked into enroll commands (default: request Host)")
	if err := fs.Parse(args); err != nil {
		return err
	}

	g, err := gateway.Open(gateway.Options{
		DataDir:   *dataDir,
		Addr:      *addr,
		ProjectID: *projectID,
		PublicURL: *publicURL,
		Version:   version,
		HubSecret: os.Getenv(brand.EnvGatewaySecret),
	})
	if err != nil {
		return err
	}
	defer g.Close()

	fmt.Printf("gateway project %s listening on %s\n", g.Project().ID, *addr)
	err = g.Serve(signalContext(), *addr)
	if err == context.Canceled {
		return nil
	}
	return err
}
