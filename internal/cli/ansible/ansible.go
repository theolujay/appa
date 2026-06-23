package ansible

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
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
{{.Host}} ansible_user={{.User}} ansible_port={{.Port}} ansible_ssh_common_args='{{.SSHCommonArgs}}'{{if .IdentityFile}} ansible_ssh_private_key_file={{.IdentityFile}}{{end}}

[appa:vars]
ansible_python_interpreter=/usr/bin/python3
ansible_remote_tmp=/tmp/.ansible
`

// inventoryData holds the template variables for
// generating an Ansible inventory file.
type inventoryData struct {
	Host          string
	User          string
	Port          int
	IdentityFile  string
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
	Quiet         bool
}

const UserDeploy = "deploy"

// GenerateInventory creates an Ansible inventory file on
// disk using the provided server configuration. It sets
// up the [appa] group with the host's SSH connection details.
func GenerateInventory(p config.ServerConfig, dest string, skipVerify bool) error {
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
		IdentityFile:  p.SSHIdentityFile,
		SSHCommonArgs: sshArgs,
	})
}

// ansibleDir returns the base path for Ansible
// configuration files. Files are embedded in the
// binary and extracted to ~/.appa/ansible/ on first use.
func ansibleDir() string {
	return ansibleExtractedDir()
}

var errAnsibleMissing = fmt.Errorf(`
Ansible is not available

Appa tried to find ansible-playbook on your PATH and then tried to
download uv to install it automatically, but neither succeeded.

Install the dependencies manually:
  curl -fsSL https://astral.sh/uv/install.sh | sh
  uv venv ~/.appa/ansible/.venv
  uv pip install 'ansible>=14.0.0'

Or install Ansible directly:
  pip install ansible
  (or: sudo apt install ansible / brew install ansible)
`)

// runCmd is a convenience wrapper around exec.Command that
// streams stdout/stderr to the terminal.
func runCmd(name string, args ...string) error {
	cmd := exec.Command(name, args...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}

// ensureVenv returns the bin directory containing Ansible binaries.
// It checks, in order:
//  1. ansible-playbook already on $PATH
//  2. managed uv venv at ~/.appa/ansible/.venv/
//  3. creates the venv (downloading uv if needed)
func ensureVenv() (string, error) {
	if path, err := exec.LookPath("ansible-playbook"); err == nil {
		return filepath.Dir(path), nil
	}

	venvDir := filepath.Join(ansibleExtractedDir(), ".venv")
	venvBinDir := filepath.Join(venvDir, "bin")
	if _, err := os.Stat(filepath.Join(venvBinDir, "ansible-playbook")); err == nil {
		return venvBinDir, nil
	}

	uvPath, err := exec.LookPath("uv")
	if err != nil {
		uvPath, err = installUV()
		if err != nil {
			return "", errAnsibleMissing
		}
	}

	if err := os.MkdirAll(ansibleExtractedDir(), 0755); err != nil {
		return "", fmt.Errorf("create ansible dir: %w", err)
	}

	fmt.Print("  Creating Python venv with Ansible...\n")
	if err := runCmd(uvPath, "venv", venvDir); err != nil {
		return "", fmt.Errorf("create uv venv: %w", err)
	}
	installCmd := exec.Command(uvPath, "pip", "install", "ansible==14.1.0")
	installCmd.Env = append(os.Environ(), "VIRTUAL_ENV="+venvDir)
	installCmd.Stdout = os.Stdout
	installCmd.Stderr = os.Stderr
	if err := installCmd.Run(); err != nil {
		return "", fmt.Errorf("install ansible: %w", err)
	}

	return venvBinDir, nil
}

// installUV downloads the uv binary to ~/.appa/bin/uv/ and returns its path.
func installUV() (string, error) {
	var triple string
	switch {
	case runtime.GOOS == "linux" && runtime.GOARCH == "amd64":
		triple = "x86_64-unknown-linux-gnu"
	case runtime.GOOS == "linux" && runtime.GOARCH == "arm64":
		triple = "aarch64-unknown-linux-gnu"
	case runtime.GOOS == "darwin" && runtime.GOARCH == "amd64":
		triple = "x86_64-apple-darwin"
	case runtime.GOOS == "darwin" && runtime.GOARCH == "arm64":
		triple = "aarch64-apple-darwin"
	default:
		return "", fmt.Errorf("unsupported platform: %s/%s", runtime.GOOS, runtime.GOARCH)
	}

	uvDir := filepath.Join(appaConfigDir(), "bin")
	uvBin := filepath.Join(uvDir, "uv")

	if _, err := os.Stat(uvBin); err == nil {
		return uvBin, nil
	}

	url := fmt.Sprintf("https://github.com/astral-sh/uv/releases/latest/download/uv-%s.tar.gz", triple)

	fmt.Print("  Downloading uv...\n")
	resp, err := http.Get(url)
	if err != nil {
		return "", fmt.Errorf("download uv: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("download uv: HTTP %d", resp.StatusCode)
	}

	gzr, err := gzip.NewReader(resp.Body)
	if err != nil {
		return "", fmt.Errorf("decompress uv: %w", err)
	}
	defer gzr.Close()

	tr := tar.NewReader(gzr)
	for {
		hdr, err := tr.Next()
		if err == io.EOF {
			return "", fmt.Errorf("uv binary not found in archive")
		}
		if err != nil {
			return "", fmt.Errorf("read uv archive: %w", err)
		}
		if filepath.Base(hdr.Name) == "uv" {
			if err := os.MkdirAll(uvDir, 0755); err != nil {
				return "", fmt.Errorf("create uv dir: %w", err)
			}
			f, err := os.OpenFile(uvBin, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0755)
			if err != nil {
				return "", fmt.Errorf("write uv: %w", err)
			}
			if _, err := io.Copy(f, tr); err != nil {
				f.Close()
				return "", fmt.Errorf("extract uv: %w", err)
			}
			f.Close()
			return uvBin, nil
		}
	}
}

// ensureDeps extracts embedded Ansible files to
// disk and installs external Ansible Galaxy roles
// from requirements.yml. It expects binDir to contain
// the ansible-galaxy binary. When quiet is true,
// progress output is suppressed (errors still surface).
func ensureDeps(binDir string, quiet bool) error {
	if err := ensureExtracted(); err != nil {
		return fmt.Errorf("extract ansible files: %w", err)
	}
	reqPath := filepath.Join(ansibleDir(), "requirements.yml")
	if _, err := os.Stat(reqPath); errors.Is(err, os.ErrNotExist) {
		return nil
	}
	cmd := exec.Command(
		filepath.Join(binDir, "ansible-galaxy"),
		"role", "install", "-r", "requirements.yml",
	)
	cmd.Dir = ansibleDir()
	if quiet {
		cmd.Stdout = io.Discard
		var stderrBuf bytes.Buffer
		cmd.Stderr = &stderrBuf
		if err := cmd.Run(); err != nil {
			return fmt.Errorf("%w: %s", err, strings.TrimSpace(stderrBuf.String()))
		}
	} else {
		cmd.Stdout = os.Stdout
		cmd.Stderr = os.Stderr
		return cmd.Run()
	}
	return nil
}

// RunPlaybook executes an Ansible playbook using
// the system's ansible-playbook command. It passes
// inventory, extra variables, and optional tags to
// control the execution flow. Output is streamed
// directly to standard output and error.
func RunPlaybook(p Playbook) error {
	binDir, err := ensureVenv()
	if err != nil {
		return err
	}
	if err := ensureDeps(binDir, p.Quiet); err != nil {
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
	cmd := exec.Command(filepath.Join(binDir, "ansible-playbook"), args...)
	cmd.Dir = ansibleDir()
	cmd.Stdin = os.Stdin

	var stdoutBuf, stderrBuf bytes.Buffer
	if p.Quiet {
		cmd.Stdout = &stdoutBuf
		cmd.Stderr = &stderrBuf
	} else {
		cmd.Stdout = os.Stdout
		cmd.Stderr = io.MultiWriter(os.Stderr, &stderrBuf)
	}
	if err := cmd.Run(); err != nil {
		var detail string
		if p.Quiet {
			detail = strings.TrimSpace(stderrBuf.String() + "\n" + stdoutBuf.String())
		} else {
			detail = strings.TrimSpace(stderrBuf.String())
		}
		return &PlaybookError{
			Playbook: p.Name,
			Err:      fmt.Errorf("%w: %s", err, detail),
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
