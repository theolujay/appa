package ssh

import (
	"errors"
	"fmt"
	"os/exec"
	"strings"
)

var ErrSSHConnectionFailed = errors.New("ssh connection failed")

type Client struct {
	User         string
	Host         string
	Port         int
	IdentityFile string
}

// Target returns a display string for the SSH target.
func Target(user, host string, port int) string {
	addr := host
	if port > 0 && port != 22 {
		addr = fmt.Sprintf("%s:%d", host, port)
	}
	return fmt.Sprintf("%s@%s", user, addr)
}

// buildArgs builds the ssh command-line arguments for a non-interactive session.
func buildArgs(user, host string, port int, identityFile string) []string {
	a := []string{
		"-o", "ConnectTimeout=5",
		"-o", "StrictHostKeyChecking=accept-new",
		"-o", "BatchMode=yes",
	}
	if port > 0 && port != 22 {
		a = append(a, "-p", fmt.Sprintf("%d", port))
	}
	if identityFile != "" {
		a = append(a, "-i", identityFile)
	}
	a = append(a, fmt.Sprintf("%s@%s", user, host))
	return a
}

// TestConnect attempts a non-interactive SSH connection to verify connectivity and credentials.
func TestConnect(user, host string, port int, identityFile ...string) error {
	idFile := ""
	if len(identityFile) > 0 {
		idFile = identityFile[0]
	}
	a := buildArgs(user, host, port, idFile)
	// run the Unix `true` utility (exits 0) to confirm connectivity
	a = append(a, "true")
	cmd := exec.Command("ssh", a...)
	out, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("%w: %s", ErrSSHConnectionFailed, string(out))
	}
	return nil
}

// RunCommand executes a non-interactive command on the remote host via SSH
// and returns its combined output.
func RunCommand(c Client, cmdLine string) (string, error) {
	a := buildArgs(c.User, c.Host, c.Port, c.IdentityFile)
	a = append(a, cmdLine)
	cmd := exec.Command("ssh", a...)
	out, err := cmd.CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("%w: %s", ErrSSHConnectionFailed, strings.TrimRight(string(out), "\n\r\t "))
	}
	return string(out), nil
}

// RunInteractiveCommand returns an exec.Cmd pre-configured for an SSH connection.
// It does not run the command immediately, allowing the caller to customize Stdin/Stdout.
func RunInteractiveCommand(c Client, remoteCmd string) *exec.Cmd {
	a := []string{
		"-o", "ConnectTimeout=10",
		"-o", "StrictHostKeyChecking=accept-new",
	}
	if c.Port > 0 && c.Port != 22 {
		a = append(a, "-p", fmt.Sprintf("%d", c.Port))
	}
	if c.IdentityFile != "" {
		a = append(a, "-i", c.IdentityFile)
	}
	a = append(a, fmt.Sprintf("%s@%s", c.User, c.Host))
	if remoteCmd != "" {
		a = append(a, remoteCmd)
	}
	cmd := exec.Command("ssh", a...)
	return cmd
}

// ResolveIP attempts to resolve a hostname to an IP address using dig or getent.
func ResolveIP(host string) (string, error) {
	cmd := exec.Command("dig", "+short", host)
	out, err := cmd.Output()
	if err != nil {
		cmd = exec.Command("getent", "hosts", host)
		out, err = cmd.Output()
		if err != nil {
			return "", fmt.Errorf("could not resolve %s: %w", host, err)
		}
	}
	ip := strings.TrimSpace(string(out))
	if ip == "" {
		return "", fmt.Errorf("no address found for %s", host)
	}
	parts := strings.Fields(ip)
	return parts[0], nil
}
