package main

import (
	"context"
	"errors"
	"fmt"

	"github.com/pleware/initagent/internal/brand"
	"github.com/pleware/initagent/internal/desk"
	"github.com/pleware/initagent/internal/frontdesk"
)

// cmdDesk runs the front desk: one loopback socket the glass talks to.
//
// It takes no flags. The address and the token are provider-grade settings —
// a token on a command line lands in ps output and in shell history — so every
// one of them arrives through the environment, and the only thing this function
// decides is what to print.
func cmdDesk(args []string) error {
	if len(args) > 0 {
		return fmt.Errorf("desk takes no arguments (configuration is %s*)", brand.EnvPrefix+"DESK_")
	}

	cfg, err := desk.LoadConfigFromEnv()
	if err != nil {
		return err
	}

	d, err := frontdesk.Open(frontdesk.Options{Config: cfg})
	if err != nil {
		if errors.Is(err, frontdesk.ErrClosed) {
			return fmt.Errorf("%w\n  copy desk.example.yaml to ~/%s/%s (or set %s)\n  %s names who answers, %s admits the glass",
				err, brand.ConfigDir, brand.DeskConfigFile, brand.EnvDeskConfig,
				brand.EnvDeskChat, brand.EnvDeskSeamToken)
		}
		return err
	}
	defer d.Close()

	fmt.Println("desk listening on", d.URL())
	for _, s := range cfg.Silences() {
		fmt.Printf("  silent: %s (%s)\n", s.Role, s.Reason)
	}

	err = d.Serve(signalContext())
	if errors.Is(err, context.Canceled) {
		return nil
	}
	return err
}
