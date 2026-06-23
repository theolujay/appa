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
	"golang.org/x/text/cases"
	"golang.org/x/text/language"
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

type ServerConfig struct {
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
	APIBaseURL      string `toml:"api_base_url,omitempty"`
	APIPort         int    `toml:"api_port,omitempty"`
}

type ProjectConfig struct {
	Name   string `toml:"name"`
	Source string `toml:"source"`
	Target string `toml:"target,omitempty"`
}

type Kind string

const (
	Server  Kind = "server"
	Project Kind = "project"
)
const ServerRemoteDir = "/opt/appa/builds"

func DefaultServer(name string) ServerConfig {
	return ServerConfig{
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
	Server:  "servers",
	Project: "projects",
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

func renameDirFor(k Kind, old, new string) error {
	oldPath := dirFor(k, old)
	newPath := filepath.Join(filepath.Dir(oldPath), new)
	return os.Rename(oldPath, newPath)
}

func load[T ServerConfig | ProjectConfig](path string) (T, error) {
	var cfg T
	_, err := toml.DecodeFile(path, &cfg)
	if err != nil {
		return cfg, fmt.Errorf("open config file: %w", err)
	}
	return cfg, nil
}

func LoadServer(name string) (ServerConfig, error) {
	return load[ServerConfig](PathFor(Server, name))
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

func SaveServer(cfg ServerConfig) error {
	dir := dirFor(Server, cfg.Name)
	if err := os.MkdirAll(dir, 0700); err != nil {
		return fmt.Errorf("create server dir: %w", err)
	}
	return writeTOML(PathFor(Server, cfg.Name), cfg)
}

func SaveProject(cfg ProjectConfig) error {
	dir := dirFor(Project, cfg.Name)
	if err := os.MkdirAll(dir, 0700); err != nil {
		return fmt.Errorf("create project dir: %w", err)
	}
	return writeTOML(PathFor(Project, cfg.Name), cfg)
}

func ListServers() ([]ServerConfig, error) {
	d := filepath.Join(baseDir(), kindDirs[Server])
	entries, err := os.ReadDir(d)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, nil
		}
		return nil, fmt.Errorf("list servers: %w", err)
	}
	var cfgs []ServerConfig
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		cfg, err := LoadServer(e.Name())
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

func ServerExists(name string) bool {
	if !ValidName(name) {
		return false
	}
	_, err := os.Stat(PathFor(Server, name))
	return err == nil
}

func ProjectExists(name string) bool {
	if !ValidName(name) {
		return false
	}
	_, err := os.Stat(PathFor(Project, name))
	return err == nil
}

func validateServer(cfg ServerConfig) error {
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
	if cfg.Target != "" && !ServerExists(cfg.Target) {
		errs = append(errs, fmt.Errorf("target server %q not found", cfg.Target))
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

	var serverCfg ServerConfig
	var projectCfg ProjectConfig
	switch kind {
	case Server:
		serverCfg, err = LoadServer(name)
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
		var oldName string
		switch kind {
		case Server:
			var cfg ServerConfig
			cfg, err = LoadServer(name)
			if err == nil {
				vErr = validateServer(cfg)
				if vErr == nil && serverCfg.Name != cfg.Name {
					vErr = renameDirFor(kind, name, cfg.Name)
					oldName = name
					name = cfg.Name
				}
			}
		case Project:
			var cfg ProjectConfig
			cfg, err = LoadProject(name)
			if err == nil {
				vErr = validateProject(cfg)
				if vErr == nil && projectCfg.Name != cfg.Name {
					vErr = renameDirFor(kind, name, cfg.Name)
					oldName = name
					name = cfg.Name
				}
			}
		}
		if vErr != nil {
			err = vErr
		}

		if err != nil {
			var rErr error
			switch kind {
			case Server:
				rErr = SaveServer(serverCfg)
				name = serverCfg.Name
			case Project:
				rErr = SaveProject(projectCfg)
				name = projectCfg.Name
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

		if oldName != "" {
			output.Success(
				"%s config %q updated and renamed to %q",
				cases.Title(language.English).String(string(kind)),
				oldName,
				name,
			)
		} else {
			output.Success(
				"%s config %q updated",
				cases.Title(language.English).String(string(kind)),
				name,
			)
		}
		return nil
	}
}

func ServerRemoteDirFor(name string) string {
	return filepath.Join(ServerRemoteDir, name)
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
