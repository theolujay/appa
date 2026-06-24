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
		Use:   "status [name]",
		Short: "Show server health and service status",
		Args:  cobra.MaximumNArgs(1),
		RunE: func(_ *cobra.Command, args []string) error {
			name := ""
			if len(args) > 0 {
				name = args[0]
			}
			if name == "" {
				if err := promptServerName(&name, "check status"); err != nil {
					return err
				}
			}
			return statusFunc(name)
		},
	}
}

func LogsCmd() *cobra.Command {
	var service string
	var tail int
	cmd := &cobra.Command{
		Use:   "logs [name]",
		Short: "Tail Appa Stack logs",
		Args:  cobra.MaximumNArgs(1),
		RunE: func(_ *cobra.Command, args []string) error {
			name := ""
			if len(args) > 0 {
				name = args[0]
			}
			if name == "" {
				if err := promptServerName(&name, "view logs"); err != nil {
					return err
				}
			}
			return logsFunc(name, service, tail)
		},
	}
	cmd.Flags().StringVarP(&service, "service", "s", "", "Filter to one service (api, db, buildkit, caddy, ui)")
	cmd.Flags().IntVarP(&tail, "tail", "n", 50, "Number of lines to show")
	return cmd
}

func RestartCmd() *cobra.Command {
	var service string
	cmd := &cobra.Command{
		Use:   "restart [name]",
		Short: "Restart Appa Stack services",
		Args:  cobra.MaximumNArgs(1),
		RunE: func(_ *cobra.Command, args []string) error {
			name := ""
			if len(args) > 0 {
				name = args[0]
			}
			if name == "" {
				if err := promptServerName(&name, "restart"); err != nil {
					return err
				}
			}
			return restartFunc(name, service)
		},
	}
	cmd.Flags().StringVarP(&service, "service", "s", "", "Restart only one service")
	return cmd
}

func UpgradeCmd() *cobra.Command {
	var version string
	cmd := &cobra.Command{
		Use:   "upgrade [name]",
		Short: "Upgrade the Appa Stack",
		Args:  cobra.MaximumNArgs(1),
		RunE: func(_ *cobra.Command, args []string) error {
			name := ""
			if len(args) > 0 {
				name = args[0]
			}
			if name == "" {
				if err := promptServerName(&name, "upgrade"); err != nil {
					return err
				}
			}
			return upgradeFunc(name, version)
		},
	}
	cmd.Flags().StringVar(&version, "version", "", "Pin to a specific version tag")
	return cmd
}

// statusFunc checks and displays the health status of a server, including
// SSH connectivity, API health, Docker Compose services, and disk usage.
func statusFunc(name string) error {
	if !config.ServerExists(name) {
		output.Error("Server %q not found", name)
		return nil
	}
	p, err := config.LoadServer(name)
	if err != nil {
		return fmt.Errorf("load config %q: %w", name, err)
	}
	if p.SSHHost == "" {
		output.Warn("No SSH target set for %q\n\tRun 'appa server set-host %s user@host'", name, name)
		return nil
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
		resp, err := hClient.Get(p.APIBaseURL + "/v1/healthcheck")
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

		fmt.Print("  Checking Docker Stack services...\n")
		checkCmd := `
			{ docker stack services appa_data --format '{{.Name}} {{.Replicas}}' 2>/dev/null; docker stack services appa --format '{{.Name}} {{.Replicas}}' 2>/dev/null; } || echo 'stack not found'
		`
		out, err := ssh.RunCommand(sClient, checkCmd)
		if err != nil {
			if errors.Is(err, ssh.ErrSSHConnectionFailed) {
				return fmt.Errorf("cannot execute command: %w", err)
			}
			return err
		}

		if strings.Contains(out, "stack not found") || out == "" {
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

// logsFunc streams logs from Docker Stack services,
// with optional service filtering and line count limits.
func logsFunc(name string, service string, tail int) error {
	if !config.ServerExists(name) {
		output.Error("Server %q not found", name)
		return nil
	}
	p, err := config.LoadServer(name)
	if err != nil {
		return fmt.Errorf("load config %q: %w", name, err)
	}
	if !p.SetupDone {
		output.Warn("Server %q not yet set up\n\tRun 'appa server setup %s'", name, name)
		return nil
	}
	clientConfig := ssh.Client{
		User:         p.SSHUser,
		Host:         p.SSHHost,
		Port:         p.SSHPort,
		IdentityFile: p.SSHIdentityFile,
		SkipVerify:   p.SSHSkipVerify,
	}
	dockerCmd := ""
	if service != "" {
		stackPrefix := "appa_"
		if service == "db" {
			stackPrefix = "appa_data_"
		}
		dockerCmd = fmt.Sprintf("docker service logs -f %s%s", stackPrefix, service)
	} else {
		dockerCmd = "docker service logs -f $(docker stack services appa_data -q) $(docker stack services appa -q) 2>/dev/null"
	}
	if tail > 0 {
		dockerCmd += fmt.Sprintf(" --tail %d", tail)
	}
	c := ssh.RunInteractiveCommand(clientConfig, dockerCmd)
	c.Stdout = os.Stdout
	c.Stderr = os.Stderr
	c.Stdin = os.Stdin
	return c.Run()
}

// restartFunc restarts Appa Stack services via Docker Stack,
// optionally limiting to a specific service.
func restartFunc(name string, service string) error {
	if !config.ServerExists(name) {
		output.Error("Server %q not found", name)
		return nil
	}
	p, err := config.LoadServer(name)
	if err != nil {
		return fmt.Errorf("load config %q: %w", name, err)
	}
	if !p.SetupDone {
		output.Warn("Server %q not yet set up\n\tRun 'appa server setup %s'", name, name)
		return nil
	}
	clientConfig := ssh.Client{
		User:         p.SSHUser,
		Host:         p.SSHHost,
		Port:         p.SSHPort,
		IdentityFile: p.SSHIdentityFile,
		SkipVerify:   p.SSHSkipVerify,
	}
	var dockerCmd string
	if service != "" {
		stackPrefix := "appa_"
		if service == "db" {
			stackPrefix = "appa_data_"
		}
		dockerCmd = fmt.Sprintf("docker service update --force %s%s", stackPrefix, service)
	} else {
		dockerCmd = "{ docker stack deploy -c /opt/appa/stack.data.yml appa_data && docker stack deploy -c /opt/appa/stack.base.yml appa; } 2>/dev/null"
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
func upgradeFunc(name string, version string) error {
	if !config.ServerExists(name) {
		output.Error("Server %q not found", name)
		return nil
	}
	p, err := config.LoadServer(name)
	if err != nil {
		return fmt.Errorf("load config %q: %w", name, err)
	}
	if !p.SetupDone {
		output.Warn("Server %q not yet set up\n\tRun 'appa server setup %s'", name, name)
		return nil
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
	pullCmd := ""
	if version != "" {
		pullCmd = fmt.Sprintf(
			`IMAGES=$(docker stack services appa_data --format '{{.Image}}' 2>/dev/null; docker stack services appa --format '{{.Image}}' 2>/dev/null) && for img in $IMAGES; do docker pull "${img%%:*}:%s"; done`,
			version,
		)
	} else {
		pullCmd = `docker stack services appa_data --format '{{.Image}}' 2>/dev/null | while read -r img; do [ -n "$img" ] && docker pull "$img"; done; docker stack services appa --format '{{.Image}}' 2>/dev/null | while read -r img; do [ -n "$img" ] && docker pull "$img"; done`
	}
	out, err := ssh.RunCommand(clientConfig, pullCmd)
	if err != nil {
		return fmt.Errorf("pull failed: %w", err)
	}
	fmt.Print(out)

	output.Success("Recreating services...")
	out, err = ssh.RunCommand(clientConfig, "{ docker stack deploy -c /opt/appa/stack.data.yml appa_data && docker stack deploy -c /opt/appa/stack.base.yml appa; } 2>/dev/null")
	if err != nil {
		return fmt.Errorf("up failed: %w", err)
	}
	fmt.Print(out)

	output.Success("Waiting for API...")
	apiURL := p.APIBaseURL + "/v1/healthcheck"
	if p.APIBaseURL == "" {
		apiURL = fmt.Sprintf("http://%s:8080/v1/healthcheck", p.SSHHost)
	}
	if err := pollHealth(apiURL, 60*time.Second); err != nil {
		return fmt.Errorf("API not healthy after upgrade: %w", err)
	}
	output.Success("Upgrade complete for %q", name)
	return nil
}
