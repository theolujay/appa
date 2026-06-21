package commands

import (
	"fmt"
	"os"
	"os/exec"
	"slices"
	"strings"

	"github.com/spf13/cobra"
	"github.com/theolujay/appa/internal/cli/config"
	"github.com/theolujay/appa/internal/cli/output"
	"github.com/theolujay/appa/internal/cli/ssh"
)

var guiEditors = map[string]string{
	// Needs --wait
	"code":          "--wait",
	"code-insiders": "--wait",
	"cursor":        "--wait",
	"zed":           "--wait",
	"atom":          "--wait",
	"pulsar":        "--wait",
	"bbedit":        "--wait",
	"coteditor":     "--wait",
	"mousepad":      "--wait",
	"geany":         "--wait",
	"notepadqq":     "--wait",
	// Needs -w
	"subl":              "-w",
	"sublime_text":      "-w",
	"gedit":             "-w",
	"gnome-text-editor": "-w",
	"mate":              "-w",
	// Needs -f
	"gvim": "-f",
}

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
	output.Success("Instance %q initialized", name)
	output.Success("  Next: appa instance set-host %s user@host", name)
	return nil
}

// instanceEditFunc handles opening an instance config in the system editor for editing.
// It supports validation and re-editing on invalid configuration.
func instanceEditFunc(_ *cobra.Command, args []string) error {
	name := args[0]
	if !config.InstanceExists(name) {
		return fmt.Errorf("%w: %s", errConfigNotFound, name)
	}

	editor := os.Getenv("APPA_EDITOR")
	if editor == "" {
		editor = os.Getenv("EDITOR")
	}
	if editor == "" {
		editor = "vim"
	}

	parts := strings.Fields(editor)
	editorBin := parts[0]
	editorArgs := parts[1:]

	waitFlag, ok := guiEditors[editorBin]
	if ok && !slices.Contains(editorArgs, waitFlag) {
		editorArgs = append(editorArgs, waitFlag)
	}

	editorPath, err := exec.LookPath(editorBin)
	if err != nil {
		return fmt.Errorf("editor %q not found on PATH", editor)
	}

	path := config.PathFor(config.Instance, name)
	output.Section("Waiting for your editor to close %q config file...", name)

	cfg, err := config.LoadInstance(name)
	if err != nil {
		return err
	}

	for {
		cmd := exec.Command(editorPath, append(editorArgs, path)...)
		cmd.Stdin = os.Stdin
		cmd.Stdout = os.Stdout
		cmd.Stderr = os.Stderr

		if editorErr := cmd.Run(); editorErr != nil {
			return fmt.Errorf("editor exited abnormally: %w", editorErr)
		}

		_, err = config.LoadInstance(name)
		if err != nil {
			if revertErr := config.SaveInstance(cfg); revertErr != nil {
				return fmt.Errorf("failed to revert to original config: %w", revertErr)
			}
			output.Error("invalid configuration: %v\n", err)
			fmt.Printf("Re-edit? [Y/n] ")
			var reply string
			fmt.Scanln(&reply)
			if reply == "n" || reply == "N" {
				return fmt.Errorf("edit aborted: config reverted to previous valid state")
			}
			continue
		}

		output.Success("Config %q updated", name)
		return nil
	}
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
