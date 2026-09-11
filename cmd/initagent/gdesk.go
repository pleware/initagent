package main

import (
	"context"
	"errors"
	"fmt"

	"github.com/pleware/initagent/internal/brand"
	"github.com/pleware/initagent/internal/gdesk"
	"github.com/pleware/initagent/internal/gdeskfront"
)

// cmdGdesk runs the glass desk: one loopback socket the glass talks to.
//
// It takes no flags. The address and the token are provider-grade settings —
// a token on a command line lands in ps output and in shell history — so every
// one of them arrives through the environment, and the only thing this function
// decides is what to print.
func cmdGdesk(args []string) error {
	if len(args) > 0 {
		return fmt.Errorf("gdesk takes no arguments (configuration is %s*)", brand.GdeskEnvPrefix)
	}

	cfg, err := gdesk.LoadConfigFromEnv()
	if err != nil {
		return err
	}

	d, err := gdeskfront.Open(gdeskfront.Options{Config: cfg})
	if err != nil {
		if errors.Is(err, gdeskfront.ErrClosed) {
			return fmt.Errorf("%w\n  copy gdesk.example.yaml to ~/%s/%s (or set %s)\n  %s names who answers, %s admits the glass",
				err, brand.ConfigDir, brand.GdeskConfigFile, brand.EnvGdeskConfig,
				brand.EnvGdeskChat, brand.EnvGdeskSeamToken)
		}
		return err
	}
	defer d.Close()

	fmt.Println("gdesk listening on", d.URL())
	for _, s := range cfg.Silences() {
		fmt.Printf("  silent: %s (%s)\n", s.Role, s.Reason)
	}

	err = d.Serve(signalContext())
	if errors.Is(err, context.Canceled) {
		return nil
	}
	return err
}
