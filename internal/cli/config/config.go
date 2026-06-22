package config

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"slices"
	"strings"

	"github.com/BurntSushi/toml"
	"github.com/theolujay/appa/internal/cli/output"
)

func ValidName(s string) bool {
	return regexp.MustCompile("^[a-zA-Z0-9_-]+$").MatchString(s)
}

func AbsPath(p string) (string, error) {
	return filepath.Abs(p)
}

func BasePath(p string) (string, error) {
	path, err := AbsPath(p)
	return filepath.Base(path), err
}

type InstanceConfig struct {
	Name            string `toml:"name"`
	SSHHost         string `toml:"ssh_host"`
	SSHUser         string `toml:"ssh_user"`
	SSHPort         int    `toml:"ssh_port"`
	SSHIdentityFile string `toml:"ssh_identity_file,omitempty"`
	SSHSkipVerify   bool   `toml:"skip_ssh_verify,omitempty"`
	Domain          string `toml:"domain"`
	OperatorUser    string `toml:"operator_user_name,omitempty"`
	CloudflareToken string `toml:"cloudflare_token"`
	SMTPHost        string `toml:"smtp_host"`
	SMTPPort        int    `toml:"smtp_port"`
	SMTPUsername    string `toml:"smtp_username"`
	SMTPPassword    string `toml:"smtp_password"`
	SetupDone       bool   `toml:"setup_done"`
	BaseAPIURL      string `toml:"base_api_url,omitempty"`
}

type ProjectConfig struct {
	Name   string `toml:"name"`
	Source string `toml:"source"`
	Target string `toml:"target,omitempty"`
}

type Kind string

const (
	Instance Kind = "instance"
	Project  Kind = "project"
)
const ServerDeployDir = "/opt/appa/builds"

func DefaultInstance(name string) InstanceConfig {
	return InstanceConfig{
		Name:     name,
		SSHUser:  "root",
		SSHPort:  22,
		SMTPPort: 587,
	}
}

func DefaultProject(name, source string) ProjectConfig {
	return ProjectConfig{
		Name:   name,
		Source: source,
	}
}

var kindDirs = map[Kind]string{
	Instance: "instances",
	Project:  "projects",
}

func baseDir() string {
	if d := os.Getenv("APPA_CONFIG_DIR"); d != "" {
		return d
	}
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".appa")
}

func dirFor(k Kind, name string) string {
	return filepath.Join(baseDir(), kindDirs[k], name)
}

func PathFor(k Kind, name string) string {
	return filepath.Join(dirFor(k, name), "config.toml")
}

func load[T InstanceConfig | ProjectConfig](path string) (T, error) {
	var cfg T
	_, err := toml.DecodeFile(path, &cfg)
	if err != nil {
		return cfg, fmt.Errorf("open config file: %w", err)
	}
	return cfg, nil
}

func LoadInstance(name string) (InstanceConfig, error) {
	return load[InstanceConfig](PathFor(Instance, name))
}

func LoadProject(name string) (ProjectConfig, error) {
	return load[ProjectConfig](PathFor(Project, name))
}

func writeTOML(path string, v any) error {
	f, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0600)
	if err != nil {
		return err
	}
	defer f.Close()
	return toml.NewEncoder(f).Encode(v)
}

func SaveInstance(cfg InstanceConfig) error {
	dir := dirFor(Instance, cfg.Name)
	if err := os.MkdirAll(dir, 0700); err != nil {
		return fmt.Errorf("create instance dir: %w", err)
	}
	return writeTOML(PathFor(Instance, cfg.Name), cfg)
}

func SaveProject(cfg ProjectConfig) error {
	dir := dirFor(Project, cfg.Name)
	if err := os.MkdirAll(dir, 0700); err != nil {
		return fmt.Errorf("create project dir: %w", err)
	}
	return writeTOML(PathFor(Project, cfg.Name), cfg)
}

func ListInstances() ([]InstanceConfig, error) {
	d := filepath.Join(baseDir(), kindDirs[Instance])
	entries, err := os.ReadDir(d)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, nil
		}
		return nil, fmt.Errorf("list instances: %w", err)
	}
	var cfgs []InstanceConfig
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		cfg, err := LoadInstance(e.Name())
		if err != nil {
			continue
		}
		cfgs = append(cfgs, cfg)
	}
	return cfgs, nil
}

func ListProjects() ([]ProjectConfig, error) {
	d := filepath.Join(baseDir(), kindDirs[Project])
	entries, err := os.ReadDir(d)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, nil
		}
		return nil, fmt.Errorf("list projects: %w", err)
	}
	var cfgs []ProjectConfig
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		cfg, err := LoadProject(e.Name())
		if err != nil {
			continue
		}
		cfgs = append(cfgs, cfg)
	}
	return cfgs, nil
}

func InstanceExists(name string) bool {
	if !ValidName(name) {
		return false
	}
	_, err := os.Stat(PathFor(Instance, name))
	return err == nil
}

func ProjectExists(name string) bool {
	if !ValidName(name) {
		return false
	}
	_, err := os.Stat(PathFor(Project, name))
	return err == nil
}

func validateInstance(cfg InstanceConfig) error {
	var errs []error
	if !ValidName(cfg.Name) {
		errs = append(errs, fmt.Errorf("invalid name %q", cfg.Name))
	}
	if cfg.SSHUser != "" && cfg.SSHHost == "" {
		errs = append(errs, fmt.Errorf("ssh_host is required when ssh_user is set"))
	}
	if cfg.SSHHost != "" && cfg.SSHUser == "" {
		errs = append(errs, fmt.Errorf("ssh_user is required when ssh_host is set"))
	}
	if cfg.SSHPort != 0 && (cfg.SSHPort < 1 || cfg.SSHPort > 65535) {
		errs = append(errs, fmt.Errorf("ssh_port %d out of range (1-65535)", cfg.SSHPort))
	}
	if cfg.OperatorUser != "" && !ValidName(cfg.OperatorUser) {
		errs = append(errs, fmt.Errorf("invalid operator_user_name %q", cfg.OperatorUser))
	}
	return errors.Join(errs...)
}

func validateProject(cfg ProjectConfig) error {
	var errs []error
	if !ValidName(cfg.Name) {
		errs = append(errs, fmt.Errorf("invalid name %q", cfg.Name))
	}
	if cfg.Source == "" {
		errs = append(errs, fmt.Errorf("source is required"))
	}
	if cfg.Target != "" && !InstanceExists(cfg.Target) {
		errs = append(errs, fmt.Errorf("target instance %q not found", cfg.Target))
	}
	return errors.Join(errs...)
}

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

func Edit(kind Kind, name string) error {

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

	path := PathFor(kind, name)
	output.Section("Waiting for your editor to close %q config file...", name)

	var instanceCfg InstanceConfig
	var projectCfg ProjectConfig
	switch kind {
	case Instance:
		instanceCfg, err = LoadInstance(name)
		if err != nil {
			return err
		}
	case Project:
		projectCfg, err = LoadProject(name)
		if err != nil {
			return err
		}
	}

	for {
		cmd := exec.Command(editorPath, append(editorArgs, path)...)
		cmd.Stdin = os.Stdin
		cmd.Stdout = os.Stdout
		cmd.Stderr = os.Stderr

		if eErr := cmd.Run(); eErr != nil {
			return fmt.Errorf("editor exited abnormally: %w", eErr)
		}

		var vErr error
		switch kind {
		case Instance:
			var cfg InstanceConfig
			cfg, err = LoadInstance(name)
			if err == nil {
				vErr = validateInstance(cfg)
			}
		case Project:
			var cfg ProjectConfig
			cfg, err = LoadProject(name)
			if err == nil {
				vErr = validateProject(cfg)
			}
		}
		if vErr != nil {
			err = vErr
		}

		if err != nil {
			var rErr error
			switch kind {
			case Instance:
				rErr = SaveInstance(instanceCfg)
			case Project:
				rErr = SaveProject(projectCfg)
			}
			if rErr != nil {
				return fmt.Errorf("failed to revert changes to %s %s config: %w", name, string(kind), rErr)
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
func ServerDirFor(name string) string {
	return filepath.Join(ServerDeployDir, name)
}
func ParseProjectSource(c string) (string, error) {

	if !strings.HasPrefix(c, "/") {
		s, err := filepath.Abs(c)
		if err != nil {
			return "", err
		}
		c = s
	}
	return c + "/", nil
}
