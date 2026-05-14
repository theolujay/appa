package pipeline

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strings"
	"sync"
	"time"

	"github.com/theolujay/appa/internal/data"
	"github.com/theolujay/appa/internal/hub"
)

// Build invokes Railpack to build the source at the given Git URL (after
// cloning the repository) into a container image. It streams every line of
// output to the hub and persists it to the database as it arrives and not
// after the build finishes. On success it returns the image tag produced.
// On failure it returns an error so the pipeline can mark the deployment
// as failed.
func (p *Pipeline) Build(ctx context.Context, id int64, source string) (string, error) {
	status := "building"
	if err := p.deployment.UpdateDeployment(id, data.DeploymentUpdate{Status: &status}); err != nil {
		return "", fmt.Errorf("failed to update status: %w", err)
	}
	p.hub.PublishStatus(id, status, "")
	imageTag := fmt.Sprintf("appa-%s", truncStr(id))

	// Check if source is a local directory (for uploads) or a git URL
	isLocal := false
	if info, err := os.Stat(source); err == nil && info.IsDir() {
		isLocal = true
	}

	var buildDir string
	if isLocal {
		buildDir = source
		logID, _ := p.deployment.AppendLog(id, "build", "using uploaded project files")
		p.hub.PublishLog(id, hub.LogMessage{ID: logID, Line: "using uploaded project files"})
	} else {
		// Create a temporary directory to clone the source repository into,
		// then clean it up afterwards
		tmpDir, err := os.MkdirTemp("", "appa-build-*")
		if err != nil {
			return "", fmt.Errorf("failed to create temp dir: %w", err)
		}
		defer os.RemoveAll(tmpDir)
		buildDir = tmpDir

		// Clone the repository into the temp directory, and stream it in logs
		msg := fmt.Sprintf("cloning %s", source)
		logID, _ := p.deployment.AppendLog(id, "build", msg)
		p.hub.PublishLog(id, hub.LogMessage{ID: logID, Line: msg})

		cloneCmd := exec.CommandContext(ctx, "git", "clone", "--quiet", "--depth=1", source, buildDir)
		cloneOut, err := cloneCmd.CombinedOutput()
		if err != nil {
			return "", fmt.Errorf("git clone failed: %s", string(cloneOut))
		}
		logID, _ = p.deployment.AppendLog(id, "build", "clone complete")
		p.hub.PublishLog(id, hub.LogMessage{ID: logID, Line: "clone complete"})
	}

	ctxWT, cancel := context.WithTimeout(ctx, 10*time.Minute)
	defer cancel()

	deployment, _ := p.deployment.GetDeployment(id)

	cmd := exec.CommandContext(ctxWT, "railpack", "build", "--name", imageTag, buildDir)

	cmd.Env = os.Environ()
	cmd.Env = append(cmd.Env, "RAILPACK_CACHE_DIR=/usr/local/share/railpack")

	if deployment.EnvVars != nil && *deployment.EnvVars != "" {
		envLines := strings.Split(*deployment.EnvVars, "\n")
		for _, line := range envLines {
			line = strings.TrimSpace(line)
			if line != "" {
				cmd.Args = append(cmd.Args, "--env", line)
			}
		}
	}
	// Attach pipes to stdout and stderr so they're read as build runs.
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return "", fmt.Errorf("failed to create stdout pipe: %w", err)
	}
	stderr, err := cmd.StderrPipe()
	if err != nil {
		return "", fmt.Errorf("failed to create stderr pipe: %w", err)
	}

	// Launches the process but doesn't wait for it to finish and returns control
	// so pipes can be read.
	if err := cmd.Start(); err != nil {
		return "", fmt.Errorf("failed to start railpack: %w", err)
	}

	// Stream both stdout and stderr to the hub and database. Merge them into
	// a single log stream to client.
	// WaitGroup allows to drain both pipes concurrently by waiting for both
	// to finish before calling cmd.Wait().
	var wg sync.WaitGroup
	wg.Add(2)
	go func() { defer wg.Done(); p.streamLogs(id, "build", stdout) }()
	go func() { defer wg.Done(); p.streamLogs(id, "build", stderr) }()
	wg.Wait()

	// Now wait for the process to exit...
	// cmd.Wait() returns an error if the process exits with a non-zero code,
	// which would mean the build failed.
	if err := cmd.Wait(); err != nil {
		return "", fmt.Errorf("railpack build failed: %w", err)
	}

	return imageTag, nil
}

func (p *Pipeline) streamLogs(id int64, phase string, r io.Reader) {
	scanner := bufio.NewScanner(r)
	for scanner.Scan() {
		line := scanner.Text()

		// First persist. If the hub publish fails for any reason, the log
		// lines still live in the database for scroll-back
		logID, err := p.deployment.AppendLog(id, phase, line)
		if err != nil {
			_ = err
		}

		p.hub.PublishLog(id, hub.LogMessage{ID: logID, Line: line})
	}
}
