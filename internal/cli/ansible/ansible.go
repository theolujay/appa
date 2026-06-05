package ansible

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"text/template"

	"github.com/theolujay/appa/internal/cli/config"
)

const inventoryTemplate = `
[appa]
{{.Host}} ansible_user={{.User}} ansible_port={{.Port}} ansible_ssh_common_args='-o StrictHostKeyChecking=accept-new'

[appa:vars]
ansible_python_interpreter=/usr/bin/python3
`

// inventoryData holds the template variables for generating an Ansible inventory file.
type inventoryData struct {
	Host string
	User string
	Port int
}

// Playbook represents the configuration for an individual ansible-playbook execution.
type Playbook struct {
	Name          string
	InventoryPath string
	Tags          string
	SkipTags      string
	ExtraVars     map[string]any
}

// GenerateInventory creates an Ansible inventory file on disk using the provided profile configuration.
// It sets up the [appa] group with the host's SSH connection details.
func GenerateInventory(p config.Profile, dest string) error {
	if err := os.MkdirAll(filepath.Dir(dest), 0700); err != nil {
		return fmt.Errorf("create inventory dir: %w", err)
	}
	tmpl, err := template.New("inventory").Parse(inventoryTemplate)
	if err != nil {
		return fmt.Errorf("parse inventory template: %w", err)
	}
	f, err := os.Create(dest)
	if err != nil {
		return fmt.Errorf("create inventory file: %w", err)
	}
	defer f.Close()
	return tmpl.Execute(f, inventoryData{
		Host: p.SSHHost,
		User: p.SSHUser,
		Port: p.SSHPort,
	})
}

// AnsibleDir returns the base path for Ansible configuration files, relative to the project root.
func AnsibleDir() string {
	return filepath.Join("deploy", "ansible")
}

// RunPlaybook executes an Ansible playbook using the system's ansible-playbook command.
// It passes inventory, extra variables, and optional tags to control the execution flow.
// Output is streamed directly to standard output and error.
//
// Example output:
//
//	`
//	PLAY [Deploy Appa Stack] ******************************************************
//	TASK [Gathering Facts] ********************************************************
//	ok: [203.0.113.10]
//	...
//	`
func RunPlaybook(p Playbook) error {
	args := []string{
		"-i",
		p.InventoryPath,
		p.Name,
	}
	if p.Tags != "" {
		args = append(args, "--tags", p.Tags)
	}
	if p.SkipTags != "" {
		args = append(args, "--skip-tags", p.SkipTags)
	}
	if p.ExtraVars != nil {
		args = append(args, "-e", toJSONString(p.ExtraVars))
	}
	cmd := exec.Command("ansible-playbook", args...)
	cmd.Dir = AnsibleDir()
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	cmd.Stdin = os.Stdin
	return cmd.Run()
}

// toJSONString is a helper that converts a map of extra variables into a JSON string
// suitable for the Ansible --extra-vars (-e) command-line flag.
func toJSONString(v map[string]any) string {
	b, _ := json.Marshal(v)
	return string(b)
}

// PlaybookPath returns the full filesystem path to a specific playbook file within the Ansible directory.
func PlaybookPath(name string) string {
	return filepath.Join(AnsibleDir(), "playbooks", name)
}
