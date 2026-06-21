package config

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"

	"github.com/BurntSushi/toml"
)

func ValidateName(s string) bool {
	r, _ := regexp.Compile("^[a-zA-Z0-9_-]+$")
	return r.MatchString(s)
}

type InstanceConfig struct {
	Name            string `toml:"name"`
	SSHHost         string `toml:"ssh_host"`
	SSHUser         string `toml:"ssh_user"`
	SSHPort         int    `toml:"ssh_port"`
	SSHIdentityFile string `toml:"ssh_identity_file,omitempty"`
	SSHSkipVerify   bool   `toml:"skip_ssh_verify,omitempty"`
	Domain          string `toml:"domain"`
	CloudflareToken string `toml:"cloudflare_token"`
	SMTPHost        string `toml:"smtp_host"`
	SMTPPort        int    `toml:"smtp_port"`
	SMTPUsername    string `toml:"smtp_username"`
	SMTPPassword    string `toml:"smtp_password"`
	SetupDone       bool   `toml:"setup_done"`
	APIURL          string `toml:"api_url,omitempty"`
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
	return cfg, fmt.Errorf("open config file: %w", err)
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
	if !ValidateName(name) {
		return false
	}
	_, err := os.Stat(PathFor(Instance, name))
	return err == nil
}

func ProjectExists(name string) bool {
	if !ValidateName(name) {
		return false
	}
	_, err := os.Stat(PathFor(Project, name))
	return err == nil
}
