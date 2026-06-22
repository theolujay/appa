package ssh

import (
	"errors"
	"fmt"
	"io"
	"os/exec"
	"strings"
)

var ErrSSHConnectionFailed = errors.New("ssh connection failed")

type Client struct {
	User         string
	Host         string
	Port         int
	IdentityFile string
	SkipVerify   bool
}

// Target returns a display string for the SSH target.
func Target(user, host string, port int) string {
	addr := host
	if port > 0 && port != 22 {
		addr = fmt.Sprintf("%s:%d", host, port)
	}
	return fmt.Sprintf("%s@%s", user, addr)
}

// sshArgs builds the base SSH arguments from the client
// configuration, excluding the remote user@host and command.
func (c Client) sshArgs() []string {
	a := []string{
		"-o", "ConnectTimeout=5",
		"-o", "BatchMode=yes",
	}
	if c.SkipVerify {
		a = append(a,
			"-o", "StrictHostKeyChecking=no",
			"-o", "UserKnownHostsFile=/dev/null",
		)
	} else {
		a = append(a, "-o", "StrictHostKeyChecking=accept-new")
	}
	if c.Port > 0 && c.Port != 22 {
		a = append(a, "-p", fmt.Sprintf("%d", c.Port))
	}
	if c.IdentityFile != "" {
		a = append(a, "-i", c.IdentityFile)
	}
	return a
}

// TestConnect attempts an SSH connection to verify connectivity and credentials.
func (c Client) TestConnect() error {
	a := c.sshArgs()
	a = append(a, fmt.Sprintf("%s@%s", c.User, c.Host), "true")
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
	a := c.sshArgs()
	a = append(a, fmt.Sprintf("%s@%s", c.User, c.Host), cmdLine)
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
	a := []string{"-o", "ConnectTimeout=10"}
	if c.SkipVerify {
		a = append(a,
			"-o", "StrictHostKeyChecking=no",
			"-o", "UserKnownHostsFile=/dev/null",
		)
	} else {
		a = append(a, "-o", "StrictHostKeyChecking=accept-new")
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
	return exec.Command("ssh", a...)
}

// RsyncOpts returns the SSH options string for rsync's -e flag,
// constructed from the client configuration.
func RsyncOpts(c Client) string {
	o := []string{"ssh"}
	if c.Port > 0 && c.Port != 22 {
		o = append(o, "-p", fmt.Sprintf("%d", c.Port))
	}
	if c.IdentityFile != "" {
		o = append(o, "-i", c.IdentityFile)
	}
	if c.SkipVerify {
		o = append(o, "-o", "StrictHostKeyChecking=no", "-o", "UserKnownHostsFile=/dev/null")
	} else {
		o = append(o, "-o", "StrictHostKeyChecking=accept-new")
	}
	o = append(o, "-o", "ConnectTimeout=10")
	return strings.Join(o, " ")
}

// Rsync copies src to the remote destPath via rsync over SSH.
// It runs the rsync command with the client's SSH configuration.
// Output is written to the provided writers (pass io.Discard to suppress).
func Rsync(c Client, src, destPath string, stdout, stderr io.Writer) error {
	target := fmt.Sprintf("%s@%s:%s", c.User, c.Host, destPath)
	args := []string{"-avz", "--mkpath", "-e", RsyncOpts(c), src, target}
	cmd := exec.Command("rsync", args...)
	cmd.Stdout = stdout
	cmd.Stderr = stderr
	return cmd.Run()
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
