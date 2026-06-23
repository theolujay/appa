package pipeline

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"os"
	"strings"
	"time"

	cerrdefs "github.com/containerd/errdefs"
	"github.com/moby/moby/api/pkg/stdcopy"
	"github.com/moby/moby/api/types/container"
	"github.com/moby/moby/client"
	"github.com/theolujay/appa/internal/data"
)

// startContainer starts a container from the given image tag and streams its logs
// to the hub and the database. It returns the host:port address of the
// running container so the router can configure Caddy to point at it.
func (p *Pipeline) startContainer(ctx context.Context, id int64) (string, error) {
	d, err := p.deployment.Get(id)
	if err != nil {
		return "", fmt.Errorf("%w: %w", errDeployFailed, err)
	}

	p.mu.Lock()
	defer p.mu.Unlock()

	res, err := p.dockerClient.ImageInspect(ctx, *d.ImageTag)
	if err != nil {
		return "", fmt.Errorf("%w: %w", errDeployFailed, err)
	}

	cPort := "3000/tcp"

	// If the image explicitly exposes ports, use the first one
	if len(res.Config.ExposedPorts) > 0 {
		for p := range res.Config.ExposedPorts {
			cPort = string(p)
			break
		}
	} else {
		cmd := ""
		if res.Config.Entrypoint != nil {
			cmd += strings.Join(res.Config.Entrypoint, "")
		}
		if res.Config.Cmd != nil {
			cmd += " " + strings.Join(res.Config.Cmd, "")
		}

		if strings.Contains(cmd, "caddy run") || strings.Contains(cmd, "http-server") {
			cPort = "80/tcp"
		}
	}

	var env []string
	if d.EnvVars != nil && *d.EnvVars != "" {
		lines := strings.Split(*d.EnvVars, "\n")
		env = make([]string, 0, len(lines))
		for _, line := range lines {
			line = strings.TrimSpace(line)
			if line != "" {
				env = append(env, line)
			}
		}
	}

	hostConfig := &container.HostConfig{
		NetworkMode: "appa_net",
		// this is set to false so logs can be inspected
		// in the future if necessary
		AutoRemove: false,
	}

	cName := fmt.Sprintf("appa-%d", d.ID)
	createResp, err := p.dockerClient.ContainerCreate(ctx, client.ContainerCreateOptions{
		Name: cName,
		Config: &container.Config{
			Image: *d.ImageTag,
			Env:   env,
		},
		HostConfig: hostConfig,
	})

	if err != nil {
		return "", fmt.Errorf("%w: %w", errDeployFailed, err)
	}

	_, err = p.dockerClient.ContainerStart(ctx, createResp.ID, client.ContainerStartOptions{})
	if err != nil {
		return "", fmt.Errorf("%w: %w", errDeployFailed, err)
	}

	addr := net.JoinHostPort(cName, strings.Split(cPort, "/")[0])

	healthy := false
	for range 60 { // wait up to 30 seconds (60 * 500ms)
		conn, err := net.DialTimeout("tcp", addr, 500*time.Millisecond)
		if err == nil {
			conn.Close()
			healthy = true
			break
		}
		time.Sleep(500 * time.Millisecond)
	}

	if !healthy {
		return "", fmt.Errorf("%w: container did not become ready on %s: %w", errDeployFailed, addr, errContainerNotReady)
	}

	go func() {
		defer p.recoverFunc(id, phaseDeploy)
		logReader, err := p.dockerClient.ContainerLogs(ctx, createResp.ID, client.ContainerLogsOptions{
			ShowStdout: true,
			ShowStderr: true,
			Follow:     true,
			Timestamps: false,
		})
		if err != nil {
			p.logLine(id, phaseDeploy, fmt.Sprintf("failed to attach container logs: %v", err))
			return
		}
		defer logReader.Close()

		// Create an in-memory pipe: pw is the write end, pr is the read end.
		// Anything written to pw can be read from pr.
		pr, pw := io.Pipe()
		// stdcopy.StdCopy runs in its own goroutine because it blocks until
		// logReader is exhausted. It strips the 8-byte Docker headers and
		// writes clean stdout bytes into pw. Stderr chunks go to io.Discard,
		// or can be directed to pw to merge them.
		go func() {
			defer func() {
				if r := recover(); r != nil {
					p.logLine(id, phaseDeploy, fmt.Sprintf("panic: %v", r))
				}
			}()
			stdcopy.StdCopy(pw, pw, logReader)
			pw.Close()
		}()

		filtered := p.caddyLogFilter(pr)
		p.streamLogs(id, phaseDeploy, filtered)
	}()

	return addr, nil
}

// stopContainer stops the Docker container, removes the associated Caddy route, and updates the deployment status to STOPPED.
func (p *Pipeline) stopContainer(dc *pipelineCtx) {
	_, err := p.deployment.Get(dc.ID)
	if err != nil {
		switch {
		case errors.Is(err, data.ErrRecordNotFound):
			dc.err = fmt.Errorf("deployment not found")
		default:
			dc.err = fmt.Errorf("%w: %w", errDeployFailed, err)
		}
		return
	}

	// Since AutoRemove was set to false in StartContainer,
	// stopping the container will NOT automatically delete it.
	// This means that there's oftentimes a possibility of the
	// container dangling and idle.
	// TODO: consider adding another container removal method
	cName := fmt.Sprintf("appa-%d", dc.ID)
	_, err = p.dockerClient.ContainerStop(
		dc.ctx, cName, client.ContainerStopOptions{},
	)
	if err != nil {
		if !cerrdefs.IsNotFound(err) {
			dc.err = fmt.Errorf("%w: %w", errContainerFailed, err)
			return
		}
	}

	err = p.router.removeRoute(dc.ID)
	if err != nil {
		dc.err = fmt.Errorf("%w: %w", errRoutingFailed, err)
	}
}

// restartContainer restarts an already built and created container, sets up log streaming, and waits for it to be healthy.
func (p *Pipeline) restartContainer(ctx context.Context, id int64) (string, error) {
	cName := fmt.Sprintf("appa-%d", id)
	_, err := p.dockerClient.ContainerRemove(ctx, cName, client.ContainerRemoveOptions{Force: true})
	if err != nil && !cerrdefs.IsNotFound(err) {
		return "", fmt.Errorf("%w: remove old container: %w", errDeployFailed, err)
	}
	return p.startContainer(ctx, id)
}

// caddyLogFilter removes Caddy HTTP access logs from an input stream.
// It is used to prevent the deployment logs from being flooded with routine request information.
func (p *Pipeline) caddyLogFilter(r io.Reader) io.Reader {
	pr, pw := io.Pipe()
	go func() {
		defer p.recoverFunc(0, "")
		s := bufio.NewScanner(r)
		for s.Scan() {
			if !strings.Contains(s.Text(), `"logger":"http.log.access`) {
				b := append([]byte(s.Text()), '\n')
				pw.Write(b)
			}
		}
		if s.Err() != nil {
			fmt.Fprintf(os.Stderr, "caddy log scanner error: %v\n", s.Err())
		}
		pw.Close()
	}()
	return pr
}
