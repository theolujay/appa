package commands

import (
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"time"

	"github.com/spf13/cobra"
	"github.com/theolujay/appa/internal/cli/ansible"
	"github.com/theolujay/appa/internal/cli/config"
	"github.com/theolujay/appa/internal/cli/output"
	"github.com/theolujay/appa/internal/cli/ssh"
	"github.com/theolujay/appa/internal/cli/tui"
	"github.com/theolujay/appa/internal/vcs"
)

// SetupCmd returns a command that performs the first-time provisioning of an Appa server.
// It runs preflight checks, generates an Ansible inventory, and executes playbooks for
// security hardening and stack deployment.
//
// Example output:
//
//	`
//	Provisioning "personal"
//	-----------------------
//	`
func SetupCmd() *cobra.Command {
	var (
		force      bool
		tags       string
		skipTags   string
		skipVerify bool
		opPubKey   string
		verbose		bool
	)

	cmd := &cobra.Command{
		Use:   "setup <name>",
		Short: "First-time provisioning of an Appa server",
		Args:  cobra.ExactArgs(1),
		RunE: func(_ *cobra.Command, args []string) error {
			return setupFunc(args, opPubKey, tags, skipTags, skipVerify, force, verbose)
		},
	}

	cmd.Flags().StringVarP(&opPubKey, "op-key", "", "", "SSH public key for server username")
	cmd.Flags().BoolVar(&force, "force", false, "Skip preflight checks")
	cmd.Flags().StringVar(&tags, "tags", "", "Only run Ansible tasks with these tags")
	cmd.Flags().StringVar(&skipTags, "skip-tags", "", "Skip Ansible tasks with these tags")
	cmd.Flags().BoolVar(&skipVerify, "skip-verify", false, "Skip SSH host key verification")
	cmd.Flags().BoolVarP(&verbose, "verbose", "v", false, "Show detailed Ansible output")

	return cmd
}

// ApplyCmd returns a command that re-applies configuration changes to an existing server.
// It is intended to be idempotent and used for updating settings like domains or firewall rules.
//
// Example output:
//
//	`
//	Applying configuration to "personal"
//	------------------------------------
//	`
func ApplyCmd() *cobra.Command {
	var (
		tags       string
		skipTags   string
		skipVerify bool
		verbose    bool
	)

	cmd := &cobra.Command{
		Use:   "apply <name>",
		Short: "Re-apply configuration changes idempotently",
		Args:  cobra.ExactArgs(1),
		RunE: func(_ *cobra.Command, args []string) error {
			return applyFunc(args, tags, skipTags, skipVerify, verbose)
		},
	}

	cmd.Flags().StringVar(&tags, "tags", "", "Only run Ansible tasks with these tags")
	cmd.Flags().StringVar(&skipTags, "skip-tags", "", "Skip Ansible tasks with these tags")
	cmd.Flags().BoolVar(&skipVerify, "skip-verify", false, "Skip SSH host key verification")
	cmd.Flags().BoolVarP(&verbose, "verbose", "v", false, "Show detailed Ansible output")
	return cmd
}

// setupFunc handles the logic for the setup command, coordinating preflight checks,
// inventory generation, and playbook execution.
func setupFunc(args []string, opPubKey, tags, skipTags string, skipVerify, force, verbose bool) error {
	name := args[0]
	if !config.ServerExists(name) {
		return fmt.Errorf("%w: %s", errConfigNotFound, name)
	}
	cfg, err := config.LoadServer(name)
	if err != nil {
		return err
	}
	if cfg.SSHHost == "" {
		return fmt.Errorf("%w: %s", errNoSSHTarget, name)
	}
	if cfg.SetupDone {
		output.Warn("Server %q has already been set up; use 'appa apply %s' for changes", name, name)
	}

	skipVerify = skipVerify || cfg.SSHSkipVerify

	if !force {
		fmt.Println("Running preflight checks...")
		preflightCmd := PreflightCmd()
		preflightCmd.SetArgs([]string{
			name,
			"--skip-verify=" + fmt.Sprintf("%t", skipVerify),
			"--no-tty",
		})
		if err = preflightCmd.Execute(); err != nil {
			return fmt.Errorf("preflight failed: use --force to skip")
		}
	}

	output.Section("Provisioning %q", name)
	tmpDir, err := os.MkdirTemp("", "appa-setup-*")
	if err != nil {
		return fmt.Errorf("create temp dir: %w", err)
	}
	defer os.RemoveAll(tmpDir)

	inventoryPath := filepath.Join(tmpDir, "inventory.ini")
	if err = ansible.GenerateInventory(cfg, inventoryPath, skipVerify); err != nil {
		return fmt.Errorf("generate inventory: %w", err)
	}

	extraVars := map[string]any{
		"appa_server_domain": cfg.Domain,
		"appa_version":         vcs.DockerTag(),
		"cloudflare_token":     cfg.CloudflareToken,
		"smtp_host":            cfg.SMTPHost,
		"smtp_port":            cfg.SMTPPort,
		"smtp_username":        cfg.SMTPUsername,
		"smtp_password":        cfg.SMTPPassword,

		"deploy_user": map[string]any{
			"name":                ansible.UserDeploy,
			"groups":              []string{"docker", "appa-admin"},
			"sudo":                "ALL=(ALL) NOPASSWD: /usr/bin/systemctl, /usr/bin/docker",
			"ssh_authorized_keys": splitKeys(opPubKey),
		},
	}

	if cfg.OperatorUser != "" {
		extraVars["operator_user"] = map[string]any{
			"name":                cfg.OperatorUser,
			"groups":              []string{"sudo", "appa-admin"},
			"sudo":                "ALL=(ALL:ALL) ALL",
			"ssh_authorized_keys": splitKeys(opPubKey),
		}
	}

	if skipVerify {
		cfg.SSHSkipVerify = true
	}

	playbook := ansible.Playbook{
		InventoryPath: inventoryPath,
		ExtraVars:     extraVars,
		Tags:          tags,
		SkipTags:      skipTags,
	}

	playbook.Name = ansible.PlaybookPath("security-hardening.yml")
	if err = runAnsible(playbook, "Applying security hardening", verbose); err != nil {
		return fmt.Errorf("apply security hardening: %w", err)
	}

	cfg.SSHUser = ansible.UserDeploy

	playbook.Name = ansible.PlaybookPath("deploy-stack.yml")
	if err = runAnsible(playbook, "Deploying Appa Stack", verbose); err != nil {
		return fmt.Errorf("deploy appa stack: %w", err)
	}

	output.Section("Waiting for Appa API to become reachable")
	apiURL := fmt.Sprintf("https://%s", cfg.Domain)
	if cfg.Domain == "" {
		apiURL = fmt.Sprintf("http://%s", cfg.SSHHost)
	}
	healthURL := apiURL + "/v1/healthcheck"
	if err = pollHealth(healthURL, 60*time.Second); err != nil {
		return fmt.Errorf("API health check failed: %w", err)
	}
	output.Success("Appa API is reachable at %s", healthURL)

	cfg.BaseAPIURL = apiURL
	cfg.SetupDone = true
	if err = config.SaveServer(cfg); err != nil {
		return fmt.Errorf("save server: %w", err)
	}

	output.Section("Setup Complete")
	output.Success("Server %q is ready!", name)
	output.Success("  API URL: %s", healthURL)
	return nil
}

// applyFunc handles the logic for the apply command, ensuring connectivity before
// re-running provisioning playbooks with specific tags.
func applyFunc(args []string, tags, skipTags string, skipVerify, verbose bool) error {
	name := args[0]
	if !config.ServerExists(name) {
		return fmt.Errorf("%w: %s", errConfigNotFound, name)
	}
	cfg, err := config.LoadServer(name)
	if err != nil {
		return err
	}
	if cfg.SSHHost == "" {
		return fmt.Errorf("%w: %s", errNoSSHTarget, name)
	}

	skipVerify = skipVerify || cfg.SSHSkipVerify

	client := ssh.Client{
		User:         cfg.SSHUser,
		Host:         cfg.SSHHost,
		Port:         cfg.SSHPort,
		IdentityFile: cfg.SSHIdentityFile,
		SkipVerify:   skipVerify,
	}
	fmt.Printf("Checking SSH connectivity to %s...\n", ssh.Target(cfg.SSHUser, cfg.SSHHost, cfg.SSHPort))
	if err := client.TestConnect(); err != nil {
		return fmt.Errorf("SSH connection failed: %w", err)
	}

	tmpDir, err := os.MkdirTemp("", "appa-apply-*")
	if err != nil {
		return fmt.Errorf("create temp dir: %w", err)
	}
	defer os.RemoveAll(tmpDir)

	inventoryPath := filepath.Join(tmpDir, "inventory.ini")
	if err := ansible.GenerateInventory(cfg, inventoryPath, skipVerify); err != nil {
		return fmt.Errorf("generate inventory: %w", err)
	}

	extraVars := map[string]any{
		"appa_server_domain": cfg.Domain,
		"appa_version":         vcs.DockerTag(),
		"cloudflare_token":     cfg.CloudflareToken,
		"smtp_host":            cfg.SMTPHost,
		"smtp_port":            cfg.SMTPPort,
		"smtp_username":        cfg.SMTPUsername,
		"smtp_password":        cfg.SMTPPassword,
	}

	playbook := ansible.Playbook{
		InventoryPath: inventoryPath,
		ExtraVars:     extraVars,
		Tags:          tags,
		SkipTags:      skipTags,
	}

	if verbose {
		output.Section("Applying configuration to %q", name)
	}

	playbook.Name = ansible.PlaybookPath("security-hardening.yml")
	if err = runAnsible(playbook, "Applying security hardening", verbose); err != nil {
		return err
	}

	playbook.Name = ansible.PlaybookPath("deploy-stack.yml")
	if err = runAnsible(playbook, "Deploying Appa Stack", verbose); err != nil {
		return err
	}

	if !verbose {
		output.Check("Configuration applied to %q", true, name)
	} else {
		output.Success("Configuration applied to %q", name)
	}
	return nil
}

// splitKeys returns a slice containing the given key,
// or an empty slice if the key is empty.
func splitKeys(s string) []string {
	if s == "" {
		return []string{}
	}
	return []string{s}
}

// runAnsible runs an Ansible playbook, showing a spinner with the given
// label when not in verbose mode, or a section header + full output when verbose.
func runAnsible(p ansible.Playbook, label string, verbose bool) error {
	if verbose {
		output.Section("%s", label)
		return ansible.RunPlaybook(p)
	}
	s := tui.StartSpinner(label)
	p.Quiet = true
	err := ansible.RunPlaybook(p)
	s.Stop(err == nil)
	return err
}

// pollHealth repeatedly checks the Appa API health endpoint until it returns 200 OK
// or the timeout is reached.
func pollHealth(url string, timeout time.Duration) error {
	deadline := time.Now().Add(timeout)
	client := &http.Client{Timeout: 5 * time.Second}
	for time.Now().Before(deadline) {
		resp, err := client.Get(url)
		if err == nil && resp.StatusCode == http.StatusOK {
			resp.Body.Close()
			return nil
		}
		if resp != nil {
			resp.Body.Close()
		}
		time.Sleep(2 * time.Second)
	}
	return fmt.Errorf("api not reachable within %v", timeout)
}
