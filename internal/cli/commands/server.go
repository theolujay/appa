package commands

import (
	"fmt"
	"os/user"
	"path/filepath"
	"strings"

	"github.com/charmbracelet/huh"
	"github.com/spf13/cobra"
	"github.com/theolujay/appa/internal/cli/config"
	"github.com/theolujay/appa/internal/cli/output"
	"github.com/theolujay/appa/internal/cli/ssh"
)

func ServerCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "server",
		Short: "Manage Appa servers",
	}

	cmd.AddCommand(serverInitCmd())
	cmd.AddCommand(serverEditCmd())
	cmd.AddCommand(serverSetHostCmd())
	cmd.AddCommand(serverListCmd())
	cmd.AddCommand(PreflightCmd())
	cmd.AddCommand(SetupCmd())
	cmd.AddCommand(ApplyCmd())
	cmd.AddCommand(StatusCmd())
	cmd.AddCommand(LogsCmd())
	cmd.AddCommand(RestartCmd())
	cmd.AddCommand(UpgradeCmd())

	return cmd
}

func serverInitCmd() *cobra.Command {
	var (
		opName string
		host string
	)

	cmd := &cobra.Command{
		Use:   "init [name]",
		Short: "Create a new server",
		Args:  cobra.MaximumNArgs(1),
		RunE: func(_ *cobra.Command, args []string) error {
			var name string
			if len(args) > 0 {
				name = args[0]
			}
			if name == "" {
				err := huh.NewInput().
					Title("What do you want to name this server?").
					Placeholder("e.g. personal").
					Value(&name).
					Validate(func(s string) error {
						if strings.TrimSpace(s) == "" {
							return fmt.Errorf("name cannot be empty")
						}
						if config.ServerExists(s) {
							return fmt.Errorf("server %q already exists", s)
						}
						return nil
					}).
					Run()
				if err != nil {
					return err
				}
			}
			return serverInitFunc([]string{name}, host, opName)
		},
	}

	cmd.Flags().StringVarP(&host, "host", "h", "", "SSH Target server (e.g. user@203.0.113.10)")
	cmd.Flags().StringVarP(&opName, "op-name", "", "", "Target server user name to set (default -> '$(whoami)')")

	return cmd
}

func serverEditCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "edit <name>",
		Short: "Edit server config in $EDITOR",
		Long: `Opens the server config in the system editor for direct TOML editing.

The editor is chosen from $APPA_EDITOR, $EDITOR, or defaults to "vi".
After saving, the file is validated. If invalid, you can re-edit or abort.`,
		Args: cobra.ExactArgs(1),
		RunE: serverEditFunc,
	}
}

func serverSetHostCmd() *cobra.Command {
	var identityFile string
	var skipVerify bool
	cmd := &cobra.Command{
		Use:   "set-host [name] [target]",
		Short: "Set SSH target for a server config",
		Args:  cobra.MaximumNArgs(2),
		RunE: func(_ *cobra.Command, args []string) error {
			var name, target string
			if len(args) > 0 {
				name = args[0]
			}
			if len(args) > 1 {
				target = args[1]
			}

			if name == "" || target == "" {
				var fields []huh.Field

				if name == "" {
					cfgs, err := config.ListServers()
					if err != nil {
						return err
					}
					if len(cfgs) == 0 {
						return fmt.Errorf("no servers found, run 'appa server init' first")
					}
					options := []huh.Option[string]{}
					for _, cfg := range cfgs {
						options = append(options, huh.NewOption(cfg.Name, cfg.Name))
					}
					fields = append(fields, huh.NewSelect[string]().
						Title("Select a server:").
						Options(options...).
						Value(&name))
				}

				if target == "" {
					fields = append(fields, huh.NewInput().
						Title("What is the SSH target?").
						Placeholder("root@203.0.113.10").
						Value(&target).
						Validate(func(s string) error {
							if strings.TrimSpace(s) == "" {
								return fmt.Errorf("target cannot be empty")
							}
							if _, _, _, err := parseTarget(s); err != nil {
								return fmt.Errorf("invalid format, use user@host or user@host:port")
							}
							return nil
						}))
				}

				err := huh.NewForm(huh.NewGroup(fields...)).Run()
				if err != nil {
					return err
				}
			}

			return serverSetHostFunc([]string{name, target}, identityFile, skipVerify)
		},
	}
	cmd.Flags().StringVarP(
		&identityFile, "identity-file", "i", "", "Path to SSH private key",
	)
	cmd.Flags().BoolVar(
		&skipVerify, "skip-verify", false, "Skip SSH host key verification",
	)
	return cmd
}

func serverListCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "ls",
		Short: "List all servers",
		Args:  cobra.NoArgs,
		RunE:  serverListFunc,
	}
}

func serverInitFunc(args []string, host, opName string) error {
	name := args[0]
	if config.ServerExists(name) {
		return fmt.Errorf("server %q already exists", name)
	}

	cfg := config.DefaultServer(name)
	if opName == "" {
		u, err := user.Current()
		if err != nil {
			return fmt.Errorf("unable to derive current username: %w", err)
		}
		opName = u.Username

	}
	cfg.SSHHost = host
	cfg.OperatorUser = opName

	if err := config.SaveServer(cfg); err != nil {
		return fmt.Errorf("save config: %w", err)
	}
	output.Success("Server %q created", name)
	if host == "" {
		output.Success("\tNext: appa server set-host %s user@host", name)
	}
	return nil
}

func serverEditFunc(_ *cobra.Command, args []string) error {
	name := args[0]
	if !config.ServerExists(name) {
		return fmt.Errorf("%w: %s", errConfigNotFound, name)
	}

	return config.Edit(config.Server, name)
}

func serverSetHostFunc(args []string, identityFile string, skipVerify bool) error {
	name, target := args[0], args[1]
	if !config.ServerExists(name) {
		return fmt.Errorf("%w: %s", errConfigNotFound, name)
	}
	user, host, port, err := parseTarget(target)
	if err != nil {
		return fmt.Errorf("invalid target: %w", err)
	}
	fmt.Printf("Testing SSH connection to %s...\n", ssh.Target(user, host, port))
	client := ssh.Client{
		User:         user,
		Host:         host,
		Port:         port,
		IdentityFile: identityFile,
		SkipVerify:   skipVerify,
	}
	if err := client.TestConnect(); err != nil {
		return fmt.Errorf("SSH connection test failed: %w", err)
	}
	cfg, err := config.LoadServer(name)
	if err != nil {
		return err
	}
	cfg.SSHUser = user
	cfg.SSHHost = host
	cfg.SSHPort = port
	if identityFile != "" {
		abs, err := filepath.Abs(identityFile)
		if err == nil {
			identityFile = abs
		}
	}
	cfg.SSHIdentityFile = identityFile
	cfg.SSHSkipVerify = skipVerify
	if err := config.SaveServer(cfg); err != nil {
		return fmt.Errorf("save server: %w", err)
	}
	// Since testing SSH connection was successful, ignore error returned, as
	// anything could cause a temporary issue and this is only best-effort.
	ip, _ := ssh.ResolveIP(host)
	if ip != "" {
		output.Success("Resolved %s -> %s", host, ip)
	}
	output.Success(
		"SSH target set for %q: %s", name, ssh.Target(user, host, port),
	)
	return nil
}

func serverListFunc(_ *cobra.Command, _ []string) error {
	cfgs, err := config.ListServers()
	if err != nil {
		return err
	}
	if len(cfgs) == 0 {
		output.Warn("No server found.")
		output.Success("Create one: appa server init <name>")
		return nil
	}
	var rows [][]string
	var dimmed []bool
	for _, p := range cfgs {
		host := p.SSHHost
		status := "pending"
		dim := true
		if host == "" {
			host = "-"
		} else if p.SetupDone {
			status = "done"
			dim = false
		}

		rows = append(rows, []string{p.Name, host, status})
		dimmed = append(dimmed, dim)
	}
	output.PrintTable([]string{"Name", "Host", "Status"}, rows, dimmed)
	return nil
}

func parseTarget(target string) (user, host string, port int, err error) {
	port = 22
	at := strings.LastIndex(target, "@")
	if at < 1 {
		return "", "", 0, errInvalidTarget
	}
	user = target[:at]
	rest := target[at+1:]
	if colon := strings.LastIndex(rest, ":"); colon > 0 {
		host = rest[:colon]
		fmt.Sscanf(rest[colon+1:], "%d", &port)
	} else {
		host = rest
	}
	if user == "" || host == "" {
		return "", "", 0, errInvalidTarget
	}
	return user, host, port, nil
}
