package config

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"github.com/BurntSushi/toml"
)

// Profile represents the configuration for an Appa instance.
type Profile struct {
	Name            string `toml:"name"`
	SSHHost         string `toml:"ssh_host"`
	SSHUser         string `toml:"ssh_user"`
	SSHPort         int    `toml:"ssh_port"`
	SSHIdentityFile string `toml:"ssh_identity_file,omitempty"`
	Domain          string `toml:"domain"`
	CloudflareToken string `toml:"cloudflare_token"`
	SMTPHost        string `toml:"smtp_host"`
	SMTPPort        int    `toml:"smtp_port"`
	SMTPUsername    string `toml:"smtp_username"`
	SMTPPassword    string `toml:"smtp_password"`
	SetupDone       bool   `toml:"setup_done"`
	APIURL          string `toml:"api_url,omitempty"`
}

// DefaultProfile returns a Profile with standard default values.
func DefaultProfile(name string) Profile {
	return Profile{
		Name:     name,
		SSHUser:  "root",
		SSHPort:  22,
		SMTPPort: 587,
	}
}

// configDir returns the base directory for Appa configuration.
// It checks the APPA_CONFIG_DIR environment variable, otherwise
// defaults to .appa in the user's home directory.
func configDir() string {
	if d := os.Getenv("APPA_CONFIG_DIR"); d != "" {
		return d
	}
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".appa")
}

// profileDir returns the directory path for a specific instance profile.
func profileDir(name string) string {
	return filepath.Join(configDir(), "instances", name)
}

// ProfilePath returns the full path to the config file for a specific profile.
func ProfilePath(name string) string {
	return filepath.Join(profileDir(name), "config.toml")
}

// Load reads a profile from disk by its name.
func Load(name string) (Profile, error) {
	var p Profile
	path := ProfilePath(name)
	_, err := toml.DecodeFile(path, &p)
	if err != nil {
		return p, fmt.Errorf("load profile %s: %w", name, err)
	}
	return p, nil
}

// Save writes a profile to disk. It creates the necessary directories
// with restricted permissions (0700) and saves the TOML file (0600).
func Save(p Profile) error {
	dir := profileDir(p.Name)
	if err := os.MkdirAll(dir, 0700); err != nil {
		return fmt.Errorf("create profile dir: %w", err)
	}
	path := ProfilePath(p.Name)
	f, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0600)
	if err != nil {
		return fmt.Errorf("open profile file: %w", err)
	}
	defer f.Close()
	return toml.NewEncoder(f).Encode(p)
}

// List returns a list of all existing instance profiles by scanning the instances directory.
func List() ([]Profile, error) {
	instancesDir := filepath.Join(configDir(), "instances")
	entries, err := os.ReadDir(instancesDir)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, nil
		}
		return nil, fmt.Errorf("list profiles: %w", err)
	}
	var profiles []Profile
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		p, err := Load(e.Name())
		if err != nil {
			continue
		}
		profiles = append(profiles, p)
	}
	return profiles, nil
}

// Exists checks if a profile with the given name exists on disk.
func Exists(name string) bool {
	_, err := os.Stat(ProfilePath(name))
	return err == nil
}
