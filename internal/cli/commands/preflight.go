package commands

import (
	"fmt"
	"net"
	"strings"
	"time"

	"github.com/spf13/cobra"
	"github.com/theolujay/appa/internal/cli/config"
	"github.com/theolujay/appa/internal/cli/output"
	"github.com/theolujay/appa/internal/cli/ssh"
)

func PreflightCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "preflight <name>",
		Short: "Run preflight checks on a target instance",
		Args:  cobra.ExactArgs(1),
		RunE:  preflightFunc,
	}
}

// preflightFunc performs comprehensive preflight checks on a target instance,
// validating SSH connectivity, OS compatibility, required ports, DNS resolution,
// Docker installation status, and configuration requirements.
func preflightFunc(_ *cobra.Command, args []string) error {
	name := args[0]
	if !config.Exists(name) {
		return fmt.Errorf("%s: %w", name, ErrProfileNotFound)
	}
	p, err := config.Load(name)
	if err != nil {
		return err
	}
	if p.SSHHost == "" {
		return fmt.Errorf("no SSH target set for %q; run 'appa instance set-host %s user@host': %w", name, name, ErrNoSSHTarget)
	}

	output.Section("Preflight Checks for %q", name)
	failures := 0
	warnings := 0

	output.Check("Profile exists", true)

	target := ssh.Target(p.SSHUser, p.SSHHost, p.SSHPort)
	fmt.Printf("  Checking SSH connectivity to %s...\n", target)
	if err := ssh.TestConnect(p.SSHUser, p.SSHHost, p.SSHPort); err != nil {
		output.Check("SSH reachable", false)
		failures++
	} else {
		output.Check("SSH reachable", true)
	}

	fmt.Print("  Checking OS compatibility...\n")
	clientConfig := ssh.Client{
		User:         p.SSHUser,
		Host:         p.SSHHost,
		Port:         p.SSHPort,
		IdentityFile: p.SSHIdentityFile,
	}
	out, err := ssh.RunCommand(clientConfig, "cat /etc/os-release 2>/dev/null | grep -i ^ID=")
	if err != nil || !strings.Contains(strings.ToLower(out), "ubuntu") {
		output.Check("OS supported (Ubuntu)", false)
		failures++
	} else {
		output.Check("OS supported (Ubuntu)", true)
	}

	fmt.Print("  Checking required ports...\n")
	ports := []int{22, 80, 443}
	allOpen := true
	for _, port := range ports {
		conn, err := net.DialTimeout("tcp", net.JoinHostPort(p.SSHHost, fmt.Sprintf("%d", port)), 3*time.Second)
		if err != nil {
			allOpen = false
			output.Warn(fmt.Sprintf("Port %d not reachable from here", port))
			warnings++
		} else {
			conn.Close()
		}
	}
	if allOpen {
		output.Check("Required ports reachable (22, 80, 443)", true)
	}

	ip, _ := ssh.ResolveIP(p.SSHHost)
	if p.Domain != "" {
		domainIP, err := net.LookupHost(p.Domain)
		if err != nil {
			output.Warn(fmt.Sprintf("Domain %q does not resolve", p.Domain))
			warnings++
		} else if ip != "" && domainIP[0] != ip {
			output.Warn(fmt.Sprintf("Domain %q resolves to %s, not instance IP %s", p.Domain, domainIP[0], ip))
			warnings++
		} else {
			output.Check(fmt.Sprintf("Domain %q resolves to instance IP", p.Domain), true)
		}
	}

	fmt.Print("  Checking for existing Docker...\n")
	out, _ = ssh.RunCommand(clientConfig, "which docker 2>/dev/null && docker --version 2>/dev/null || echo 'not found'")
	if strings.Contains(out, "not found") {
		output.Check("Docker not installed (clean host)", true)
	} else {
		output.Warn(fmt.Sprintf("Docker already installed: %s", strings.TrimSpace(out)))
		warnings++
	}

	if p.CloudflareToken == "" {
		output.Warn("Cloudflare API token not set (needed for wildcard TLS)")
		warnings++
	} else {
		output.Check("Cloudflare API token set", true)
	}
	if p.SMTPHost == "" {
		output.Warn("SMTP not configured (needed for email notifications)")
		warnings++
	} else {
		output.Check("SMTP configured", true)
	}

	fmt.Println()
	if failures > 0 {
		output.Error("%d failure(s), %d warning(s)", failures, warnings)
		return fmt.Errorf("preflight failed")
	} else if warnings > 0 {
		output.Success("All critical checks passed (%d warning(s))", warnings)
	} else {
		output.Success("All checks passed")
	}
	return nil
}
