package ansible

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"text/template"

	"github.com/theolujay/appa/internal/cli/config"
)

// PlaybookError is returned when an Ansible playbook execution fails.
// It wraps the underlying execution error (e.g., *exec.ExitError)
// while providing context about which playbook failed.
type PlaybookError struct {
	Playbook string
	Err      error
}

func (e *PlaybookError) Error() string {
	return fmt.Sprintf("playbook (%q) failed: %v", filepath.Base(e.Playbook), e.Err)
}

func (e *PlaybookError) Unwrap() error {
	return e.Err
}

const inventoryTemplate = `
[appa]
{{.Host}}
ansible_user={{.User}}
ansible_port={{.Port}}
ansible_ssh_common_args='{{.SSHCommonArgs}}'

[appa:vars]
ansible_python_interpreter=/usr/bin/python3
`

// inventoryData holds the template variables for
// generating an Ansible inventory file.
type inventoryData struct {
	Host          string
	User          string
	Port          int
	SSHCommonArgs string
}

// Playbook represents the configuration for an
// individual ansible-playbook execution.
type Playbook struct {
	Name          string
	InventoryPath string
	Tags          string
	SkipTags      string
	ExtraVars     map[string]any
}

const UserDeploy = "deploy"

// GenerateInventory creates an Ansible inventory file on
// disk using the provided instance configuration. It sets
// up the [appa] group with the host's SSH connection details.
func GenerateInventory(p config.InstanceConfig, dest string, skipVerify bool) error {
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

	sshArgs := "-o StrictHostKeyChecking=accept-new"
	if skipVerify {
		sshArgs = "-o StrictHostKeyChecking=no -o UserKnownHostsFile=/dev/null"
	}

	return tmpl.Execute(f, inventoryData{
		Host:          p.SSHHost,
		User:          p.SSHUser,
		Port:          p.SSHPort,
		SSHCommonArgs: sshArgs,
	})
}

// ansibleDir returns the base path for Ansible
// configuration files. Files are embedded in the
// binary and extracted to ~/.appa/ansible/ on first use.
func ansibleDir() string {
	return ansibleExtractedDir()
}

// ensureDeps extracts embedded Ansible files to
// disk and installs external Ansible Galaxy roles
// from requirements.yml.
func ensureDeps() error {
	if err := ensureExtracted(); err != nil {
		return fmt.Errorf("extract ansible files: %w", err)
	}
	reqPath := filepath.Join(ansibleDir(), "requirements.yml")
	if _, err := os.Stat(reqPath); errors.Is(err, os.ErrNotExist) {
		return nil
	}
	cmd := exec.Command(
		"ansible-galaxy", "role", "install", "-r", "requirements.yml",
	)
	cmd.Dir = ansibleDir()
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}

// RunPlaybook executes an Ansible playbook using
// the system's ansible-playbook command. It passes
// inventory, extra variables, and optional tags to
// control the execution flow. Output is streamed
// directly to standard output and error.
func RunPlaybook(p Playbook) error {
	if err := ensureDeps(); err != nil {
		return fmt.Errorf("install galaxy deps: %w", err)
	}
	args := []string{
		"--inventory",
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
	cmd.Dir = ansibleDir()
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout

	var errBuf bytes.Buffer
	cmd.Stderr = io.MultiWriter(os.Stderr, &errBuf)
	if err := cmd.Run(); err != nil {
		return &PlaybookError{
			Playbook: p.Name,
			Err:      fmt.Errorf("%w: %s", err, strings.TrimSpace(errBuf.String())),
		}
	}
	return nil
}

// toJSONString is a helper that converts a map of
// extra variables into a JSON string suitable for
// the Ansible --extra-vars (-e) command-line flag.
func toJSONString(v map[string]any) string {
	b, _ := json.Marshal(v)
	return string(b)
}

// PlaybookPath returns the full filesystem path to a
// specific playbook file within the Ansible directory.
func PlaybookPath(name string) string {
	return filepath.Join(ansibleDir(), "playbooks", name)
}
