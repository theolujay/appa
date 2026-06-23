package commands

import (
	"errors"
	"fmt"
	"net"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/huh"
	"github.com/spf13/cobra"
	"github.com/theolujay/appa/internal/cli/config"
	"github.com/theolujay/appa/internal/cli/output"
	"github.com/theolujay/appa/internal/cli/ssh"
	"github.com/theolujay/appa/internal/cli/tui"
)

func PreflightCmd() *cobra.Command {
	var (
		skipVerify bool
		noTTY      bool
		serverName string
	)
	cmd := &cobra.Command{
		Use:   "preflight [name]",
		Short: "Run preflight checks on a target server",
		Args:  cobra.MaximumNArgs(1),
		RunE: func(_ *cobra.Command, args []string) error {
			if len(args) == 0 && serverName == "" && !noTTY {
				cfgs, err := config.ListServers()
				if err != nil {
					return err
				}
				if len(cfgs) == 0 {
					output.Warn("No servers found; run 'appa server init' first")
					return nil
				}
				options := make([]huh.Option[string], len(cfgs))
				for i, cfg := range cfgs {
					options[i] = huh.NewOption(cfg.Name, cfg.Name)
				}
				if err := huh.NewSelect[string]().
					Title("Select a server to check:").
					Options(options...).
					Value(&serverName).
					Run(); err != nil {
					return err
				}
			}
			if len(args) > 0 {
				serverName = args[0]
			}
			if serverName == "" {
				output.Error("Server name is required")
				return nil
			}
			return preflightFunc(serverName, skipVerify, noTTY)
		},
	}
	cmd.Flags().BoolVar(
		&skipVerify, "skip-verify", false, "Skip SSH host key verification",
	)
	cmd.Flags().BoolVar(
		&noTTY, "no-tty", false, "Run in non-interactive mode (no TUI)",
	)
	return cmd
}

// preflightFunc performs comprehensive preflight checks on a target server,
// validating SSH connectivity, OS compatibility, required ports, DNS resolution,
// Docker installation status, and configuration requirements.
func preflightFunc(name string, skipVerify bool, noTTY bool) error {
	if !config.ServerExists(name) {
		return fmt.Errorf("%w: %s", errConfigNotFound, name)
	}
	p, err := config.LoadServer(name)
	if err != nil {
		return err
	}
	if p.SSHHost == "" {
		return fmt.Errorf("no SSH target set for %q; run 'appa server set-host %s user@host': %w", name, name, errNoSSHTarget)
	}

	clientConfig := ssh.Client{
		User:         p.SSHUser,
		Host:         p.SSHHost,
		Port:         p.SSHPort,
		IdentityFile: p.SSHIdentityFile,
		SkipVerify:   skipVerify || p.SSHSkipVerify,
	}

	checks := []tui.Check{
		{
			Label: "Server exists",
			Fn: func() (bool, string, bool) {
				return true, "", false
			},
		},
		{
			Label: "SSH reachable",
			Fn: func() (bool, string, bool) {
				err := clientConfig.TestConnect()
				if err != nil {
					return false, err.Error(), false
				}
				return true, "", false
			},
		},
		{
			Label: "OS supported (Ubuntu)",
			Fn: func() (bool, string, bool) {
				out, err := ssh.RunCommand(clientConfig, "cat /etc/os-release 2>/dev/null | grep -i ^ID=")
				if err != nil || !strings.Contains(strings.ToLower(out), "ubuntu") {
					return false, "", false
				}
				return true, "", false
			},
		},
		{
			Label: fmt.Sprintf("Required ports reachable [%d, 80, 443]", p.SSHPort),
			Fn: func() (bool, string, bool) {
				var errs []error
				ports := []int{p.SSHPort, 80, 443}
				for _, port := range ports {
					conn, err := net.DialTimeout("tcp", net.JoinHostPort(p.SSHHost, fmt.Sprintf("%d", port)), 3*time.Second)
					if err != nil {
						errs = append(errs, fmt.Errorf("port %d not reachable from here", port))
						continue
					}
					conn.Close()
				}
				if len(errs) > 0 {
					err := errors.Join(errs...)
					return true, fmt.Sprintf("%v", err), true
				}
				return true, "", false
			},
		},
		{
			Label: "Domain resolves to server IP",
			Fn: func() (bool, string, bool) {
				if p.Domain == "" {
					return true, "No domain configured", true
				}
				ip, _ := ssh.ResolveIP(p.SSHHost)
				domainIP, err := net.LookupHost(p.Domain)
				if err != nil {
					return true, fmt.Sprintf("Domain %q does not resolve", p.Domain), true
				} else if ip != "" && domainIP[0] != ip {
					return true, fmt.Sprintf("Resolves to %s, not %s", domainIP[0], ip), true
				}
				return true, "", false
			},
		},
		{
			Label: "Docker not already installed (clean host)",
			Fn: func() (bool, string, bool) {
				out, _ := ssh.RunCommand(clientConfig, "which docker 2>/dev/null && docker --version 2>/dev/null || echo 'not found'")
				if strings.Contains(out, "not found") {
					return true, "", false
				}
				return true, strings.TrimSpace(out), true
			},
		},
		{
			Label: "Cloudflare API token set",
			Fn: func() (bool, string, bool) {
				if p.CloudflareToken == "" {
					return true, "Token not set (needed for wildcard TLS)", true
				}
				return true, "", false
			},
		},
		{
			Label: "SMTP configured",
			Fn: func() (bool, string, bool) {
				if p.SMTPHost == "" {
					return true, "SMTP not configured", true
				}
				return true, "", false
			},
		},
	}

	if noTTY {
		return runChecksPlain(checks)
	}

	model := tui.NewPreflightModel(checks)
	pProg := tea.NewProgram(model)
	m, err := pProg.Run()
	if err != nil {
		return fmt.Errorf("error running preflight TUI: %w", err)
	}

	if pm, ok := m.(*tui.PreflightModel); ok {
		if pm.Failures > 0 {
			return fmt.Errorf("preflight failed")
		}
	}

	return nil
}

func runChecksPlain(checks []tui.Check) error {
	failures := 0
	for _, c := range checks {
		ok, info, warn := c.Fn()
		switch {
		case !ok:
			output.Check(c.Label, false)
			failures++
		case warn:
			output.Warn("%s", c.Label)
		default:
			output.Check(c.Label, true)
		}
		if info != "" {
			fmt.Printf("  %s\n", info)
		}
	}
	if failures > 0 {
		return fmt.Errorf("preflight failed with %d failure(s)", failures)
	}
	return nil
}
