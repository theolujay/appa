package commands

import (
	"fmt"
	"strings"

	"github.com/spf13/cobra"
	"github.com/theolujay/appa/internal/cli/config"
	"github.com/theolujay/appa/internal/cli/output"
	"github.com/theolujay/appa/internal/cli/ssh"
)

// InstanceCmd returns the root command for managing Appa instances.
// It provides subcommands for initializing, editing, setting hosts, and listing configs.
func InstanceCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "instance",
		Short: "Manage Appa instances",
	}

	cmd.AddCommand(instanceInitCmd())
	cmd.AddCommand(instanceEditCmd())
	cmd.AddCommand(instanceSetHostCmd())
	cmd.AddCommand(instanceListCmd())

	return cmd
}

func instanceInitCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "init <name>",
		Short: "Create a new instance",
		Args:  cobra.ExactArgs(1),
		RunE:  instanceInitFunc,
	}
}

func instanceEditCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "edit <name>",
		Short: "Edit instance config in $EDITOR",
		Long: `Opens the instance config in the system editor for direct TOML editing.

The editor is chosen from $APPA_EDITOR, $EDITOR, or defaults to "vi".
After saving, the file is validated. If invalid, you can re-edit or abort.`,
		Args: cobra.ExactArgs(1),
		RunE: instanceEditFunc,
	}
}

func instanceSetHostCmd() *cobra.Command {
	var identityFile string
	var skipVerify bool
	cmd := &cobra.Command{
		Use:   "set-host <name> <target>",
		Short: "Set SSH target for an instance config",
		Args:  cobra.ExactArgs(2),
		RunE: func(_ *cobra.Command, args []string) error {
			return instanceSetHostFunc(args, identityFile, skipVerify)
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

func instanceListCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "list",
		Short: "List all instances",
		Args:  cobra.NoArgs,
		RunE:  instanceListFunc,
	}
}

// instanceInitFunc handles the creation of a new instance with validation.
func instanceInitFunc(_ *cobra.Command, args []string) error {
	name := args[0]
	if !config.InstanceExists(name) {
		return fmt.Errorf("%w: %s", errConfigNotFound, name)
	}

	cfg := config.DefaultInstance(name)
	cfg.Name = name
	if err := config.SaveInstance(cfg); err != nil {
		return fmt.Errorf("save config: %w", err)
	}
	output.Success("Instance %q created", name)
	output.Success("\tNext: appa instance set-host %s user@host", name)
	return nil
}

// instanceEditFunc handles opening an instance config in the system editor for editing.
// It supports validation and re-editing on invalid configuration.
func instanceEditFunc(_ *cobra.Command, args []string) error {
	name := args[0]
	if !config.InstanceExists(name) {
		return fmt.Errorf("%w: %s", errConfigNotFound, name)
	}

	return config.Edit(config.Instance, name)
}

// instanceSetHostFunc sets the SSH target for an instance and tests the connection.
func instanceSetHostFunc(args []string, identityFile string, skipVerify bool) error {
	name, target := args[0], args[1]
	if !config.InstanceExists(name) {
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
	cfg, err := config.LoadInstance(name)
	if err != nil {
		return err
	}
	cfg.SSHUser = user
	cfg.SSHHost = host
	cfg.SSHPort = port
	cfg.SSHIdentityFile = identityFile
	cfg.SSHSkipVerify = skipVerify
	if err := config.SaveInstance(cfg); err != nil {
		return fmt.Errorf("save instance: %w", err)
	}
	// Since testing SSH connetion was successful, ignore error returned, as
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

// instanceListFunc displays all instances and their current status.
func instanceListFunc(_ *cobra.Command, _ []string) error {
	cfgs, err := config.ListInstances()
	if err != nil {
		return err
	}
	if len(cfgs) == 0 {
		fmt.Println("No instance found.")
		fmt.Println("  Create one: appa instance init <name>")
		return nil
	}
	var rows [][]string
	for _, p := range cfgs {
		host := p.SSHHost
		if host == "" {
			host = "-"
		}
		status := "pending"
		if p.SetupDone {
			status = "done"
		} else if p.SSHHost != "" {
			status = "host set"
		}
		rows = append(rows, []string{p.Name, host, status})
	}
	output.PrintTable([]string{"Name", "Host", "Status"}, rows)
	return nil
}

// parseTarget splits an SSH connection string into its components.
// It supports formats like "user@host" and "user@host:port", defaulting to port 22.
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
