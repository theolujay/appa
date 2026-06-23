package commands

import (
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/spf13/cobra"
	"github.com/theolujay/appa/internal/cli/config"
	"github.com/theolujay/appa/internal/cli/output"
	"github.com/theolujay/appa/internal/cli/ssh"
)

func StatusCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "status <name>",
		Short: "Show server health and service status",
		Args:  cobra.ExactArgs(1),
		RunE:  statusFunc,
	}
}

func LogsCmd() *cobra.Command {
	var service string
	var tail int
	cmd := &cobra.Command{
		Use:   "logs <name>",
		Short: "Tail Appa Stack logs",
		Args:  cobra.ExactArgs(1),
		RunE: func(_ *cobra.Command, args []string) error {
			return logsFunc(args, service, tail)
		},
	}
	cmd.Flags().StringVarP(&service, "service", "s", "", "Filter to one service (api, db, buildkit, caddy, ui)")
	cmd.Flags().IntVarP(&tail, "tail", "n", 50, "Number of lines to show")
	return cmd
}

func RestartCmd() *cobra.Command {
	var service string
	cmd := &cobra.Command{
		Use:   "restart <name>",
		Short: "Restart Appa Stack services",
		Args:  cobra.ExactArgs(1),
		RunE: func(_ *cobra.Command, args []string) error {
			return restartFunc(args, service)
		},
	}
	cmd.Flags().StringVarP(&service, "service", "s", "", "Restart only one service")
	return cmd
}

func UpgradeCmd() *cobra.Command {
	var version string
	cmd := &cobra.Command{
		Use:   "upgrade <name>",
		Short: "Upgrade the Appa Stack",
		Args:  cobra.ExactArgs(1),
		RunE: func(_ *cobra.Command, args []string) error {
			return upgradeFunc(args, version)
		},
	}
	cmd.Flags().StringVar(&version, "version", "", "Pin to a specific version tag")
	return cmd
}

// statusFunc checks and displays the health status of a server, including
// SSH connectivity, API health, Docker Compose services, and disk usage.
func statusFunc(_ *cobra.Command, args []string) error {
	name := args[0]
	if !config.ServerExists(name) {
		return fmt.Errorf("%w: %s", errConfigNotFound, name)
	}
	p, err := config.LoadServer(name)
	if err != nil {
		return fmt.Errorf("load config %q: %w", name, err)
	}
	if p.SSHHost == "" {
		return fmt.Errorf("%s: %w", name, errNoSSHTarget)
	}

	output.Section("Status for %q", name)

	sClient := ssh.Client{
		User:         p.SSHUser,
		Host:         p.SSHHost,
		Port:         p.SSHPort,
		IdentityFile: p.SSHIdentityFile,
		SkipVerify:   p.SSHSkipVerify,
	}

	output.Check("SSH target", p.SSHHost != "")
	sshOK := sClient.TestConnect() == nil
	output.Check("SSH reachable", sshOK)

	switch {
	case p.SetupDone && sshOK:
		fmt.Print("  Checking API health...\n")
		hClient := &http.Client{Timeout: 5 * time.Second}
		resp, err := hClient.Get(p.BaseAPIURL + "/v1/healthcheck")
		if err != nil {
			return fmt.Errorf("unable to reach API: %w", err)
		}
		defer resp.Body.Close()

		if resp.StatusCode == http.StatusOK {
			body, err := io.ReadAll(resp.Body)
			if err != nil {
				return fmt.Errorf("failed to read API response body: %w", err)
			}
			output.Check("API status:\n%s", true, string(body))
		} else {
			output.Check("API healthy", false)
		}

		fmt.Print("  Checking Docker Compose services...\n")
		checkCmd := `
			docker compose -f /opt/appa/compose.yml ps --format '{{.Name}} {{.Status}}' 2>/dev/null || echo 'compose not found'
		`
		out, err := ssh.RunCommand(sClient, checkCmd)
		if err != nil {
			if errors.Is(err, ssh.ErrSSHConnectionFailed) {
				return fmt.Errorf("cannot execute command: %w", err)
			}
			return err
		}

		if strings.Contains(out, "compose not found") || out == "" {
			output.Check("Appa Stack running", false)
		} else {
			output.Check("Appa Stack services", true)
			fmt.Println(strings.TrimSpace(out))
		}

		fmt.Print("  Checking disk usage...\n")
		diskOut, err := ssh.RunCommand(sClient,
			"df -h / | awk 'NR==2 {print \"  Used: \" $3 \" / \" $2 \" (\" $5 \")\"}'",
		)
		if err != nil {
			return fmt.Errorf("unable to get status: %w", err)
		}
		if diskOut != "" {
			fmt.Println(strings.TrimSpace(diskOut))
		}
	case !sshOK:
		output.Warn("Cannot check further: SSH not reachable")
	default:
		output.Warn("Server not yet set up; run 'appa setup %s'", name)
	}
	return nil
}

// logsFunc streams logs from Docker Compose services,
// with optional service filtering and line count limits.
func logsFunc(args []string, service string, tail int) error {
	name := args[0]
	if !config.ServerExists(name) {
		return fmt.Errorf("%w: %s", errConfigNotFound, name)
	}
	p, err := config.LoadServer(name)
	if err != nil {
		return fmt.Errorf("load config %q: %w", name, err)
	}
	if p.SSHHost == "" {
		return fmt.Errorf("%s: %w", name, errNoSSHTarget)
	}
	clientConfig := ssh.Client{
		User:         p.SSHUser,
		Host:         p.SSHHost,
		Port:         p.SSHPort,
		IdentityFile: p.SSHIdentityFile,
		SkipVerify:   p.SSHSkipVerify,
	}
	dockerCmd := "docker compose -f /opt/appa/compose.yml logs -f"
	if tail > 0 {
		dockerCmd += fmt.Sprintf(" -n %d", tail)
	}
	if service != "" {
		dockerCmd += " " + service
	}
	c := ssh.RunInteractiveCommand(clientConfig, dockerCmd)
	c.Stdout = os.Stdout
	c.Stderr = os.Stderr
	c.Stdin = os.Stdin
	return c.Run()
}

// restartFunc restarts Appa Stack services via Docker Compose,
// optionally limiting to a specific service.
func restartFunc(args []string, service string) error {
	name := args[0]
	if !config.ServerExists(name) {
		return fmt.Errorf("%w: %s", errConfigNotFound, name)
	}
	p, err := config.LoadServer(name)
	if err != nil {
		return fmt.Errorf("load config %q: %w", name, err)
	}
	if p.SSHHost == "" {
		return fmt.Errorf("%s: %w", name, errNoSSHTarget)
	}
	clientConfig := ssh.Client{
		User:         p.SSHUser,
		Host:         p.SSHHost,
		Port:         p.SSHPort,
		IdentityFile: p.SSHIdentityFile,
		SkipVerify:   p.SSHSkipVerify,
	}
	dockerCmd := "docker compose -f /opt/appa/compose.yml restart"
	if service != "" {
		dockerCmd += " " + service
	}
	output.Success("Restarting services on %q...", name)
	out, err := ssh.RunCommand(clientConfig, dockerCmd)
	if err != nil {
		return fmt.Errorf("restart failed: %w", err)
	}
	fmt.Print(out)
	output.Success("Restart complete")
	return nil
}

// upgradeFunc upgrades the Appa Stack by pulling latest images
// and recreating services, with optional pinning to a specific
// version tag. It waits for the API to become healthy after upgrade.
func upgradeFunc(args []string, version string) error {
	name := args[0]
	if !config.ServerExists(name) {
		return fmt.Errorf("%w: %s", errConfigNotFound, name)
	}
	p, err := config.LoadServer(name)
	if err != nil {
		return fmt.Errorf("load config %q: %w", name, err)
	}
	if p.SSHHost == "" {
		return fmt.Errorf("%s: %w", name, errNoSSHTarget)
	}
	clientConfig := ssh.Client{
		User:         p.SSHUser,
		Host:         p.SSHHost,
		Port:         p.SSHPort,
		IdentityFile: p.SSHIdentityFile,
		SkipVerify:   p.SSHSkipVerify,
	}

	output.Section("Upgrading %q", name)

	output.Success("Pulling latest images...")
	pullCmd := "docker compose -f /opt/appa/compose.yml pull"
	if version != "" {
		pullCmd = fmt.Sprintf(
			`IMAGES=$(docker compose -f /opt/appa/compose.yml config --images 2>/dev/null || docker compose -f /opt/appa/compose.yml images -q) && for img in $IMAGES; do docker pull "${img%%:*}:%s"; done`,
			version,
		)
	}
	out, err := ssh.RunCommand(clientConfig, pullCmd)
	if err != nil {
		return fmt.Errorf("pull failed: %w", err)
	}
	fmt.Print(out)

	output.Success("Recreating services...")
	out, err = ssh.RunCommand(clientConfig, "docker compose -f /opt/appa/compose.yml up -d")
	if err != nil {
		return fmt.Errorf("up failed: %w", err)
	}
	fmt.Print(out)

	output.Success("Waiting for API...")
	apiURL := p.BaseAPIURL + "/v1/healthcheck"
	if p.BaseAPIURL == "" {
		apiURL = fmt.Sprintf("http://%s:8080/v1/healthcheck", p.SSHHost)
	}
	if err := pollHealth(apiURL, 60*time.Second); err != nil {
		return fmt.Errorf("API not healthy after upgrade: %w", err)
	}
	output.Success("Upgrade complete for %q", name)
	return nil
}
