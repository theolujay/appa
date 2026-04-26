package pipeline

import (
	"bufio"
	"fmt"
	"io"
	"os/exec"
	"sync"
)

// Build invokes Railpack to build the source at the given Git URL into a
// container image. It streams every line of output to the hub and persists
// it to the database as it arrives and not after the build finishes.
// On success it returns the image tag produced. On failure it returns an
// error so the pipeline can mark the deployment as failed.
func (p *Pipeline) Build(deploymentID, source string) (string, error) {
	// Mark as 'building' immediately
	if err := p.store.UpdateDeploymentStatus(deploymentID, "building"); err != nil {
		return "", fmt.Errorf("failed to update status: %w", err)
	}
	imageTag := fmt.Sprintf("appa-%s", truncateDeploymentID(deploymentID))

	// Construct Railway build command. `--image` flag tels Railpack what to
	// name the resulting image. The `source` arg is the Git URL.
	// Railway clones the repo, detects the runtime, and builds the image.
	cmd := exec.Command("railpack", "build", "--image", imageTag, source)

	// Attack pipes to stdout and stderr so they're read as build runs.
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
	// Use a WaitGroup so we can drain both pipes concurrently and then
	// wait for both to finish before calling cmd.Wait().
	var wg sync.WaitGroup
	wg.Add(2)
	go func() { defer wg.Done(); p.streamLogs(deploymentID, "build", stdout) }()
	go func() { defer wg.Done(); p.streamLogs(deploymentID, "build", stderr) }()
	// Blocker here until both goroutines have finished draining their pipes, so
	// logs aren't truncated when Build completes
	wg.Wait()

	// Now wait for the process to exit...
	// cmd.Wait() returns an error if the process exits with a non-zero code,
	// which would mean the build failed.
	if err := cmd.Wait(); err != nil {
		return "", fmt.Errorf("railpack build failed: %w", err)
	}

	return imageTag, nil
}

func (p *Pipeline) streamLogs(deploymentID, phase string, r io.Reader) {
	scanner := bufio.NewScanner(r)
	for scanner.Scan() {
		line := scanner.Text()

		// First persist. If the hub publish fails for any reason, the log
		// lines still live in the database for scroll-back
		if err := p.store.AppendLog(deploymentID, phase, line); err != nil {
			// Issues with logging shouldn't abort the build, so don't return here
			_ = err
		}

		p.hub.Publish(deploymentID, line)
	}
}
