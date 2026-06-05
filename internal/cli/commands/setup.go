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
)

// SetupCmd returns a command that performs the first-time provisioning of an Appa instance.
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
	var force bool
	var tags string
	var skipTags string

	cmd := &cobra.Command{
		Use:   "setup <name>",
		Short: "First-time provisioning of an Appa instance",
		Args:  cobra.ExactArgs(1),
		RunE: func(_ *cobra.Command, args []string) error {
			return setupFunc(args, force, tags, skipTags)
		},
	}

	cmd.Flags().BoolVar(&force, "force", false, "Skip preflight checks")
	cmd.Flags().StringVar(&tags, "tags", "", "Only run Ansible tasks with these tags")
	cmd.Flags().StringVar(&skipTags, "skip-tags", "", "Skip Ansible tasks with these tags")
	return cmd
}

// ApplyCmd returns a command that re-applies configuration changes to an existing instance.
// It is intended to be idempotent and used for updating settings like domains or firewall rules.
//
// Example output:
//
//	`
//	Applying configuration to "personal"
//	------------------------------------
//	`
func ApplyCmd() *cobra.Command {
	var tags string
	var skipTags string

	cmd := &cobra.Command{
		Use:   "apply <name>",
		Short: "Re-apply configuration changes idempotently",
		Args:  cobra.ExactArgs(1),
		RunE: func(_ *cobra.Command, args []string) error {
			return applyFunc(args, tags, skipTags)
		},
	}

	cmd.Flags().StringVar(&tags, "tags", "", "Only run Ansible tasks with these tags")
	cmd.Flags().StringVar(&skipTags, "skip-tags", "", "Skip Ansible tasks with these tags")
	return cmd
}

// setupFunc handles the logic for the setup command, coordinating preflight checks,
// inventory generation, and playbook execution.
func setupFunc(args []string, force bool, tags, skipTags string) error {
	name := args[0]
	if !config.Exists(name) {
		return fmt.Errorf("profile %q not found", name)
	}
	p, err := config.Load(name)
	if err != nil {
		return err
	}
	if p.SSHHost == "" {
		return fmt.Errorf("no SSH target set for %q", name)
	}
	if p.SetupDone {
		output.Warn("Instance %q has already been set up; use 'appa apply %s' for changes", name, name)
	}

	if !force {
		fmt.Println("Running preflight checks...")
		preflightCmd := PreflightCmd()
		preflightCmd.SetArgs([]string{name})
		if err := preflightCmd.Execute(); err != nil {
			return fmt.Errorf("preflight failed; use --force to skip")
		}
	}

	output.Section("Provisioning %q", name)
	tmpDir, err := os.MkdirTemp("", "appa-setup-*")
	if err != nil {
		return fmt.Errorf("create temp dir: %w", err)
	}
	defer os.RemoveAll(tmpDir)

	inventoryPath := filepath.Join(tmpDir, "inventory.ini")
	if err := ansible.GenerateInventory(p, inventoryPath); err != nil {
		return fmt.Errorf("generate inventory: %w", err)
	}

	extraVars := map[string]any{
		"appa_domain":      p.Domain,
		"cloudflare_token": p.CloudflareToken,
		"smtp_host":        p.SMTPHost,
		"smtp_port":        p.SMTPPort,
		"smtp_username":    p.SMTPUsername,
		"smtp_password":    p.SMTPPassword,
	}

	playbook := ansible.Playbook{
		InventoryPath: inventoryPath,
		ExtraVars:     extraVars,
		Tags:          tags,
		SkipTags:      skipTags,
	}

	output.Section("Applying security hardening")
	secPlaybook := ansible.PlaybookPath("security-hardening.yml")
	playbook.Name = secPlaybook
	if err := ansible.RunPlaybook(playbook); err != nil {
		return fmt.Errorf("security hardening playbook failed: %w", err)
	}

	output.Section("Deploying Appa Stack")

	stackPlaybook := ansible.PlaybookPath("deploy-stack.yml")
	playbook.Name = stackPlaybook
	if err := ansible.RunPlaybook(playbook); err != nil {
		return fmt.Errorf("deploy stack playbook failed: %w", err)
	}

	output.Section("Waiting for Appa API to become reachable")
	apiURL := fmt.Sprintf("https://%s/v1/healthcheck", p.Domain)
	if p.Domain == "" {
		apiURL = fmt.Sprintf("http://%s:8080/v1/healthcheck", p.SSHHost)
	}
	if err := pollHealth(apiURL, 60*time.Second); err != nil {
		return fmt.Errorf("API health check failed: %w", err)
	}
	output.Success("Appa API is reachable at %s", apiURL)

	p.SetupDone = true
	p.APIURL = apiURL
	if err := config.Save(p); err != nil {
		return fmt.Errorf("save profile: %w", err)
	}

	output.Section("Setup Complete")
	output.Success("Instance %q is ready!", name)
	output.Success("  API URL: %s", apiURL)
	return nil
}

// applyFunc handles the logic for the apply command, ensuring connectivity before
// re-running provisioning playbooks with specific tags.
func applyFunc(args []string, tags, skipTags string) error {
	name := args[0]
	if !config.Exists(name) {
		return fmt.Errorf("profile %q not found", name)
	}
	p, err := config.Load(name)
	if err != nil {
		return err
	}
	if p.SSHHost == "" {
		return fmt.Errorf("no SSH target set for %q", name)
	}

	fmt.Printf("Checking SSH connectivity to %s...\n", ssh.Target(p.SSHUser, p.SSHHost, p.SSHPort))
	if err := ssh.TestConnect(p.SSHUser, p.SSHHost, p.SSHPort); err != nil {
		return fmt.Errorf("SSH connection failed: %w", err)
	}

	tmpDir, err := os.MkdirTemp("", "appa-apply-*")
	if err != nil {
		return fmt.Errorf("create temp dir: %w", err)
	}
	defer os.RemoveAll(tmpDir)

	inventoryPath := filepath.Join(tmpDir, "inventory.ini")
	if err := ansible.GenerateInventory(p, inventoryPath); err != nil {
		return fmt.Errorf("generate inventory: %w", err)
	}

	extraVars := map[string]any{
		"appa_domain":      p.Domain,
		"cloudflare_token": p.CloudflareToken,
		"smtp_host":        p.SMTPHost,
		"smtp_port":        p.SMTPPort,
		"smtp_username":    p.SMTPUsername,
		"smtp_password":    p.SMTPPassword,
	}

	playbook := ansible.Playbook{
		InventoryPath: inventoryPath,
		ExtraVars:     extraVars,
		Tags:          tags,
		SkipTags:      skipTags,
	}

	output.Section("Applying configuration to %q", name)
	secPlaybook := ansible.PlaybookPath("security-hardening.yml")
	playbook.Name = secPlaybook
	if err := ansible.RunPlaybook(playbook); err != nil {
		return fmt.Errorf("security hardening playbook failed: %w", err)
	}

	stackPlaybook := ansible.PlaybookPath("deploy-stack.yml")
	playbook.Name = stackPlaybook
	if err := ansible.RunPlaybook(playbook); err != nil {
		return fmt.Errorf("deploy stack playbook failed: %w", err)
	}

	output.Success("Configuration applied to %q", name)
	return nil
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
	return fmt.Errorf("API not reachable within %v", timeout)
}
