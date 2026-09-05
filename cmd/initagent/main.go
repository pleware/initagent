// Command initagent is the single binary: hub, gateway, device agent, fleet CLI, and MCP.
package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"os"
	"os/signal"
	"runtime"
	"strings"
	"syscall"
	"text/tabwriter"
	"time"

	"github.com/pleware/initagent/internal/agent"
	"github.com/pleware/initagent/internal/brand"
	"github.com/pleware/initagent/internal/fleet"
	"github.com/pleware/initagent/internal/hub"
	"github.com/pleware/initagent/internal/mcp"
	"github.com/pleware/initagent/internal/offering"
	"github.com/pleware/initagent/internal/updater"
)

var version = "0.1.0-dev" // overridden at release time via -ldflags

func usageText() string {
	b, n, cfg := brand.Binary, brand.Name, brand.ConfigDir
	return strings.NewReplacer(
		"{{bin}}", b,
		"{{name}}", n,
		"{{cfg}}", cfg,
	).Replace(`{{name}} — control all your machines from one place.

Usage:
  {{bin}} serve [--addr :4200] [--data-dir ~/{{cfg}}] [--gateway-url URL] [--offering selfhost|hosted]
                                                             Run the hub (web UI + API)
  {{bin}} serve --tls-domain d.com --tls-email you@d.com   Run the hub with automatic HTTPS (Let's Encrypt)
  {{bin}} gateway [--addr :4201] [--data-dir ~/{{cfg}}] [--project prj-…] [--public-url URL]
                                                             Run the project gateway (enroll + tasks)
  {{bin}} agent enroll --hub URL --token TOKEN            Enroll this device with a hub
  {{bin}} agent run                                       Run the device agent (foreground)
  {{bin}} agent install-service                           Install + start the agent as a service
  {{bin}} fleet login --hub URL --token API_TOKEN         Save fleet CLI credentials
  {{bin}} fleet devices                                   List devices
  {{bin}} fleet sessions [device]                         List sessions (fleet-wide or one device)
  {{bin}} fleet new DEVICE NAME [--cwd DIR] [--cmd CMD]   Create a session
  {{bin}} fleet run DEVICE -- CMD...                      Run a command and print output
  {{bin}} fleet send DEVICE SESSION TEXT                  Type into a session (presses Enter)
  {{bin}} fleet read DEVICE SESSION [--lines N]           Read a session's recent output
  {{bin}} fleet kill DEVICE SESSION                       Kill a session
  {{bin}} mcp                                             Run the MCP server (stdio) for coding agents
  {{bin}} update [--check]                                Install or check the latest verified stable release
  {{bin}} rollback                                        Restore the previous verified binary
  {{bin}} version                                         Print version
`)
}

func main() {
	log.SetFlags(log.Ltime)
	if len(os.Args) < 2 {
		fmt.Print(usageText())
		os.Exit(2)
	}
	var err error
	switch os.Args[1] {
	case "serve":
		err = cmdServe(os.Args[2:])
	case "gateway":
		err = cmdGateway(os.Args[2:])
	case "agent":
		err = cmdAgent(os.Args[2:])
	case "fleet":
		err = cmdFleet(os.Args[2:])
	case "mcp":
		err = cmdMCP()
	case "update":
		err = cmdUpdate(os.Args[2:])
	case "rollback":
		err = cmdRollback()
	case "version", "--version", "-v":
		fmt.Println(brand.Binary, version)
	case "help", "--help", "-h":
		fmt.Print(usageText())
	default:
		fmt.Fprintf(os.Stderr, "%s: unknown command %q\n\n%s", brand.Binary, os.Args[1], usageText())
		os.Exit(2)
	}
	if err != nil {
		fmt.Fprintln(os.Stderr, brand.Binary+":", err)
		os.Exit(1)
	}
}

func cmdUpdate(args []string) error {
	fs := flag.NewFlagSet("update", flag.ExitOnError)
	check := fs.Bool("check", false, "only report whether an update is available")
	fs.Parse(args)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
	defer cancel()
	release, err := updater.Latest(ctx, brand.ReleaseSource, runtime.GOOS, runtime.GOARCH)
	if err != nil {
		return err
	}
	if !updater.IsNewer(release.Version, version) {
		fmt.Printf("%s %s is already current\n", brand.Binary, version)
		return nil
	}
	if *check {
		fmt.Printf("update available: %s -> %s\n", version, release.Version)
		return nil
	}
	if err := updater.Install(ctx, release, os.Getenv(brand.EnvWindowsTask)); err != nil {
		return err
	}
	fmt.Printf("updated %s -> %s; restart %s to use it\n", version, release.Version, brand.Name)
	return nil
}

func cmdRollback() error {
	if err := updater.Rollback(os.Getenv(brand.EnvWindowsTask)); err != nil {
		return err
	}
	fmt.Printf("previous %s version restored; restart to use it\n", brand.Name)
	return nil
}

func signalContext() context.Context {
	ctx, cancel := context.WithCancel(context.Background())
	ch := make(chan os.Signal, 1)
	signal.Notify(ch, os.Interrupt, syscall.SIGTERM)
	go func() {
		<-ch
		cancel()
	}()
	return ctx
}

func cmdServe(args []string) error {
	fs := flag.NewFlagSet("serve", flag.ExitOnError)
	addr := fs.String("addr", ":4200", "listen address (ignored when --tls-domain is set)")
	dataDir := fs.String("data-dir", "", "data directory (default ~/"+brand.ConfigDir+")")
	gatewayURL := fs.String("gateway-url", "", "project gateway URL for enroll (required to add workers)")
	databaseURL := fs.String("database-url", os.Getenv(brand.EnvDatabaseURL), "Postgres connection string; empty = SQLite under --data-dir")
	offeringFlag := fs.String("offering", "", "hub offering: selfhost or hosted (default: "+brand.OfferingFile+" in --data-dir, else selfhost)")
	tlsDomain := fs.String("tls-domain", "", "enable automatic HTTPS (Let's Encrypt) for this domain; serves :443 + :80")
	tlsEmail := fs.String("tls-email", "", "contact email for Let's Encrypt (expiry notices)")
	fs.Parse(args)

	if *tlsDomain != "" && *tlsEmail == "" {
		return fmt.Errorf("--tls-email is required with --tls-domain (Let's Encrypt needs a contact address)")
	}

	resolvedDir, err := offering.Dir(*dataDir)
	if err != nil {
		return err
	}
	fileBody, filePresent, err := offering.ReadFile(resolvedDir)
	if err != nil {
		return err
	}
	kind, err := offering.Resolve(*offeringFlag, os.Getenv(brand.EnvOffering), fileBody, filePresent)
	if err != nil {
		return err
	}
	if err := offering.RequireStart(kind, *databaseURL); err != nil {
		return err
	}
	log.Printf("offering %s", kind)

	srv, err := hub.NewServer(hub.Options{
		Addr:         *addr,
		DataDir:      resolvedDir,
		Version:      version,
		GithubRepo:   brand.ReleaseSource,
		TLSDomain:    *tlsDomain,
		TLSEmail:     *tlsEmail,
		UI:           uiFS(),
		GatewayURL:   *gatewayURL,
		DatabaseURL:  *databaseURL,
		Offering:     kind,
		ResendAPIKey: os.Getenv(brand.EnvResendAPIKey),
		MailFrom:     os.Getenv(brand.EnvMailFrom),
	})
	if err != nil {
		return err
	}
	return srv.Run(signalContext())
}

func cmdAgent(args []string) error {
	if len(args) < 1 {
		return fmt.Errorf("usage: %s agent <enroll|run|install-service>", brand.Binary)
	}
	switch args[0] {
	case "enroll":
		fs := flag.NewFlagSet("enroll", flag.ExitOnError)
		hubURL := fs.String("hub", "", "hub URL, e.g. http://192.168.1.10:4200")
		token := fs.String("token", "", "enrollment token from the hub UI")
		fs.Parse(args[1:])
		if *hubURL == "" || *token == "" {
			return fmt.Errorf("both --hub and --token are required")
		}
		cfg, err := agent.Enroll(*hubURL, *token)
		if err != nil {
			return err
		}
		fmt.Printf("enrolled as device %s with hub %s\n", cfg.DeviceId, cfg.HubURL)
		return nil
	case "run":
		cfg, err := agent.LoadConfig()
		if err != nil {
			return err
		}
		return agent.New(cfg, version).Run(signalContext())
	case "install-service":
		if _, err := agent.LoadConfig(); err != nil {
			return err
		}
		if err := agent.InstallService(); err != nil {
			return err
		}
		fmt.Println("agent service installed and started")
		return nil
	default:
		return fmt.Errorf("unknown agent subcommand %q", args[0])
	}
}

func cmdMCP() error {
	client, err := fleet.NewFromEnv()
	if err != nil {
		return err
	}
	return mcp.Serve(client, version)
}

func cmdFleet(args []string) error {
	if len(args) < 1 {
		return fmt.Errorf("usage: %s fleet <login|devices|sessions|new|run|send|read|kill>", brand.Binary)
	}
	sub, rest := args[0], args[1:]

	if sub == "login" {
		fs := flag.NewFlagSet("login", flag.ExitOnError)
		hubURL := fs.String("hub", "", "hub URL")
		token := fs.String("token", "", "API token (create one in the UI under Settings)")
		fs.Parse(rest)
		if *hubURL == "" || *token == "" {
			return fmt.Errorf("both --hub and --token are required")
		}
		client := fleet.New(*hubURL, *token)
		if _, err := client.Devices(); err != nil {
			return fmt.Errorf("could not talk to hub: %w", err)
		}
		if err := fleet.SaveConfig(fleet.ClientConfig{HubURL: *hubURL, Token: *token}); err != nil {
			return err
		}
		fmt.Println("fleet credentials saved")
		return nil
	}

	client, err := fleet.NewFromEnv()
	if err != nil {
		return err
	}

	switch sub {
	case "devices":
		devices, err := client.Devices()
		if err != nil {
			return err
		}
		tw := tabwriter.NewWriter(os.Stdout, 2, 4, 2, ' ', 0)
		fmt.Fprintln(tw, "NAME\tID\tOS/ARCH\tSTATUS\tLAST SEEN")
		for _, d := range devices {
			status := "offline"
			if d.Online {
				status = "online"
			}
			if d.IsHub {
				status += " (hub)"
			}
			lastSeen := "-"
			if d.LastSeen > 0 {
				lastSeen = time.Unix(d.LastSeen, 0).Format("Jan 2 15:04")
			}
			fmt.Fprintf(tw, "%s\t%s\t%s/%s\t%s\t%s\n", d.Name, d.Id, d.OS, d.Arch, status, lastSeen)
		}
		return tw.Flush()

	case "sessions":
		var sessions []fleet.Session
		if len(rest) > 0 {
			d, err := client.ResolveDevice(rest[0])
			if err != nil {
				return err
			}
			sessions, err = client.Sessions(d.Id)
			if err != nil {
				return err
			}
			for i := range sessions {
				sessions[i].DeviceName = d.Name
			}
		} else {
			sessions, err = client.FleetSessions()
			if err != nil {
				return err
			}
		}
		tw := tabwriter.NewWriter(os.Stdout, 2, 4, 2, ' ', 0)
		fmt.Fprintln(tw, "SESSION\tDEVICE\tKIND\tSTATUS")
		for _, s := range sessions {
			kind := s.Kind
			if kind == "" {
				kind = "terminal"
			}
			fmt.Fprintf(tw, "%s\t%s\t%s\t%s\n", s.Name, s.DeviceName, kind, s.Status)
		}
		return tw.Flush()

	case "new":
		fs := flag.NewFlagSet("new", flag.ExitOnError)
		cwd := fs.String("cwd", "", "working directory")
		cmd := fs.String("cmd", "", "command to run in the session")
		kind := fs.String("kind", "", "label, e.g. claude")
		if len(rest) < 2 {
			return fmt.Errorf("usage: %s fleet new DEVICE NAME [--cwd DIR] [--cmd CMD]", brand.Binary)
		}
		fs.Parse(rest[2:])
		d, err := client.ResolveDevice(rest[0])
		if err != nil {
			return err
		}
		if err := client.CreateSession(d.Id, rest[1], *cwd, *cmd, *kind); err != nil {
			return err
		}
		fmt.Printf("session %q created on %s\n", rest[1], d.Name)
		return nil

	case "run":
		if len(rest) < 2 {
			return fmt.Errorf("usage: %s fleet run DEVICE -- CMD...", brand.Binary)
		}
		d, err := client.ResolveDevice(rest[0])
		if err != nil {
			return err
		}
		cmdArgs := rest[1:]
		if cmdArgs[0] == "--" {
			cmdArgs = cmdArgs[1:]
		}
		command := ""
		for i, a := range cmdArgs {
			if i > 0 {
				command += " "
			}
			command += a
		}
		res, err := client.Run(d.Id, command, "", 0)
		if err != nil {
			return err
		}
		fmt.Print(res.Stdout)
		fmt.Fprint(os.Stderr, res.Stderr)
		if res.ExitCode != 0 {
			os.Exit(res.ExitCode)
		}
		return nil

	case "send":
		if len(rest) < 3 {
			return fmt.Errorf("usage: %s fleet send DEVICE SESSION TEXT", brand.Binary)
		}
		d, err := client.ResolveDevice(rest[0])
		if err != nil {
			return err
		}
		return client.SendInput(d.Id, rest[1], rest[2], true)

	case "read":
		fs := flag.NewFlagSet("read", flag.ExitOnError)
		lines := fs.Int("lines", 200, "lines of scrollback")
		if len(rest) < 2 {
			return fmt.Errorf("usage: %s fleet read DEVICE SESSION [--lines N]", brand.Binary)
		}
		fs.Parse(rest[2:])
		d, err := client.ResolveDevice(rest[0])
		if err != nil {
			return err
		}
		out, err := client.ReadOutput(d.Id, rest[1], *lines)
		if err != nil {
			return err
		}
		fmt.Print(out)
		return nil

	case "kill":
		if len(rest) < 2 {
			return fmt.Errorf("usage: %s fleet kill DEVICE SESSION", brand.Binary)
		}
		d, err := client.ResolveDevice(rest[0])
		if err != nil {
			return err
		}
		return client.KillSession(d.Id, rest[1])

	default:
		return fmt.Errorf("unknown fleet subcommand %q", sub)
	}
}
