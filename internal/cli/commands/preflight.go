package commands

import (
	"fmt"
	"net"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/spf13/cobra"
	"github.com/theolujay/appa/internal/cli/config"
	"github.com/theolujay/appa/internal/cli/ssh"
	"github.com/theolujay/appa/internal/cli/tui"
)

func PreflightCmd() *cobra.Command {
	var skipVerify bool
	cmd := &cobra.Command{
		Use:   "preflight <name>",
		Short: "Run preflight checks on a target instance",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return preflightFunc(cmd, args, skipVerify)
		},
	}
	cmd.Flags().BoolVar(
		&skipVerify, "skip-verify", false, "Skip SSH host key verification",
	)
	return cmd
}

// preflightFunc performs comprehensive preflight checks on a target instance,
// validating SSH connectivity, OS compatibility, required ports, DNS resolution,
// Docker installation status, and configuration requirements.
func preflightFunc(_ *cobra.Command, args []string, skipVerify bool) error {
	name := args[0]
	if !config.InstanceExists(name) {
		return fmt.Errorf("%w: %s", errConfigNotFound, name)
	}
	p, err := config.LoadInstance(name)
	if err != nil {
		return err
	}
	if p.SSHHost == "" {
		return fmt.Errorf("no SSH target set for %q; run 'appa instance set-host %s user@host': %w", name, name, errNoSSHTarget)
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
			Label: "Instance exists",
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
			Label: "Required ports reachable (22, 80, 443)",
			Fn: func() (bool, string, bool) {
				ports := []int{22, 80, 443}
				for _, port := range ports {
					conn, err := net.DialTimeout("tcp", net.JoinHostPort(p.SSHHost, fmt.Sprintf("%d", port)), 3*time.Second)
					if err != nil {
						return true, fmt.Sprintf("Port %d not reachable from here", port), true
					}
					conn.Close()
				}
				return true, "", false
			},
		},
		{
			Label: "Domain resolves to instance IP",
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
