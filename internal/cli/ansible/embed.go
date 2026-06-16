package ansible

import (
	"embed"
	"io/fs"
	"os"
	"path/filepath"
)

//go:generate sh -c "rm -rf _embed && mkdir _embed && cp -r ../../../deploy/ansible/. _embed && rm -rf _embed/.venv _embed/.ansible"
//go:embed all:_embed
var ansibleFS embed.FS

// appaConfigDir returns the root directory for
// Appa configuration and data.
func appaConfigDir() string {
	if d := os.Getenv("APPA_CONFIG_DIR"); d != "" {
		return d
	}
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".appa")
}

// ansibleExtractedDir returns the path where
// embedded Ansible files are stored.
func ansibleExtractedDir() string {
	return filepath.Join(appaConfigDir(), "ansible")
}

// ensureExtracted extracts embedded Ansible
// assetsto the local filesystem if missing.
func ensureExtracted() error {
	dir := ansibleExtractedDir()
	marker := filepath.Join(dir, "playbooks", "deploy-stack.yml")
	if _, err := os.Stat(marker); err == nil {
		return nil
	}
	return fs.WalkDir(ansibleFS, "_embed", func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		relPath, _ := filepath.Rel("_embed", path)
		target := filepath.Join(dir, relPath)
		if d.IsDir() {
			return os.MkdirAll(target, 0755)
		}
		data, err := ansibleFS.ReadFile(path)
		if err != nil {
			return err
		}
		mode := fs.FileMode(0644)
		if info, err := d.Info(); err == nil && info.Mode()&0100 != 0 {
			mode = 0755
		}
		return os.WriteFile(target, data, mode)
	})
}
