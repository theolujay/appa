package pipeline

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"net"
	"strings"
	"time"

	"github.com/moby/moby/api/pkg/stdcopy"
	"github.com/moby/moby/api/types/container"
	"github.com/moby/moby/client"
	"github.com/theolujay/appa/internal/data"
	"github.com/theolujay/appa/internal/hub"
)

// StartContainer starts a container from the given image tag and streams its logs
// to the hub and the database. It returns the host:port address of the
// running container so the router can configure Caddy to point at it.
func (p *Pipeline) StartContainer(ctx context.Context, id int64, imageTag string) (string, error) {
	status := data.DEPLOYING
	deployment, err := p.deployment.UpdateAndGet(id, data.DeploymentUpdate{Status: &status})
	if err != nil {
		return "", fmt.Errorf("failed to update status: %w", err)
	}
	p.hub.PublishStatus(id, status, "")

	p.mu.Lock()
	defer p.mu.Unlock()

	hostPort, err := getFreePort()
	if err != nil {
		return "", fmt.Errorf("filed to find free port: %w", err)
	}

	imageInspectResult, err := p.dockerClient.ImageInspect(ctx, imageTag)
	if err != nil {
		return "", fmt.Errorf("failed to inspect image: %w", err)
	}

	containerPortStr := "3000/tcp"

	// If the image explicitly exposes ports, use the first one
	if len(imageInspectResult.Config.ExposedPorts) > 0 {
		for port := range imageInspectResult.Config.ExposedPorts {
			containerPortStr = string(port)
			break
		}
	} else {
		cmd := ""
		if imageInspectResult.Config.Entrypoint != nil {
			cmd += strings.Join(imageInspectResult.Config.Entrypoint, " ")
		}
		if imageInspectResult.Config.Cmd != nil {
			cmd += " " + strings.Join(imageInspectResult.Config.Cmd, " ")
		}

		if strings.Contains(cmd, "caddy run") || strings.Contains(cmd, "http-server") {
			containerPortStr = "80/tcp"
		}
	}

	var env []string
	if deployment.EnvVars != nil && *deployment.EnvVars != "" {
		lines := strings.Split(*deployment.EnvVars, "\n")
		env = make([]string, 0, len(lines))
		for _, line := range lines {
			line = strings.TrimSpace(line)
			if line != "" {
				env = append(env, line)
			}
		}
	}

	containerName := fmt.Sprintf("appa-%d", id)

	hostConfig := &container.HostConfig{
		NetworkMode: "appa_net",
		AutoRemove:  false,
	}

	createResp, err := p.dockerClient.ContainerCreate(ctx, client.ContainerCreateOptions{
		Name: containerName,
		Config: &container.Config{
			Image: imageTag,
			Env:   env,
		},
		HostConfig: hostConfig,
	})

	if err != nil {
		return "", fmt.Errorf("failed to create container: %w", err)
	}

	_, err = p.dockerClient.ContainerStart(ctx, createResp.ID, client.ContainerStartOptions{})
	if err != nil {
		return "", fmt.Errorf("failed to start container: %w", err)
	}

	address := net.JoinHostPort(containerName, strings.Split(containerPortStr, "/")[0])

	msg := fmt.Sprintf("waiting for container %s to respond...", address)
	logID, _ := p.deployment.AppendLog(id, phaseDeploy, msg)
	p.hub.PublishLog(id, hub.LogMessage{ID: logID, Line: msg})

	healthy := false
	for range 60 { // wait up to 30 seconds (60 * 500ms)
		conn, err := net.DialTimeout("tcp", address, 500*time.Millisecond)
		if err == nil {
			conn.Close()
			healthy = true
			break
		}
		time.Sleep(500 * time.Millisecond)
	}

	if !healthy {
		msg := "container failed to start"
		logID, _ := p.deployment.AppendLog(id, phaseDeploy, msg)
		p.hub.PublishLog(id, hub.LogMessage{ID: logID, Line: msg})
		return "", fmt.Errorf("container did not respond on port %d", hostPort)
	} else {
		msg := "container is healthy and accepting connections"
		logID, _ := p.deployment.AppendLog(id, phaseDeploy, msg)
		p.hub.PublishLog(id, hub.LogMessage{ID: logID, Line: msg})
	}

	go func() {
		logReader, err := p.dockerClient.ContainerLogs(ctx, createResp.ID, client.ContainerLogsOptions{
			ShowStdout: true,
			ShowStderr: true,
			Follow:     true,
			Timestamps: false,
		})
		if err != nil {
			p.deployment.AppendLog(id, phaseDeploy, fmt.Sprintf("failed to attach container logs: %v", err))
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
			stdcopy.StdCopy(pw, pw, logReader)
			pw.Close()
		}()

		filtered := caddyLogFilter(pr)
		p.streamLogs(id, phaseDeploy, filtered)
	}()

	return address, nil
}

// StopContainer stops the Docker container, removes the associated Caddy route, and updates the deployment status to STOPPED.
func (p *Pipeline) StopContainer(id int64) error {
	URL := ""
	imageTag := ""
	status := data.STOPPED

	// Since AutoRemove: true was set in StartContainer,
	// stopping the container will automatically delete it.
	containerName := fmt.Sprintf("appa-%d", id)
	_, err := p.dockerClient.ContainerStop(
		context.Background(), containerName, client.ContainerStopOptions{},
	)
	if err != nil {
		if !strings.Contains(err.Error(), "No such container") {
			return fmt.Errorf("failed to stop container %s: %w", containerName, err)
		}
	}
	err = p.router.RemoveRoute(id)
	if err != nil {
		fmt.Printf("failed to remove caddy route for %d: %v\n", id, err)
	}

	_, err = p.deployment.UpdateAndGet(
		id,
		data.DeploymentUpdate{
			URL:      &URL,
			Status:   &status,
			ImageTag: &imageTag,
		},
	)
	if err != nil {
		return err
	}

	msg := "deployment stopped by user"
	logID, _ := p.deployment.AppendLog(id, phaseCancel, msg)
	p.hub.PublishLog(id, hub.LogMessage{ID: logID, Line: msg})
	p.hub.PublishStatus(id, status, "")

	return nil
}

// getFreePort finds and returns an available TCP port by binding to port 0 on localhost.
func getFreePort() (int, error) {
	// the port number is automatically chosen with 0 as port in address parameter
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return 0, err
	}
	defer ln.Close()
	return ln.Addr().(*net.TCPAddr).Port, nil
}

// caddyLogFilter removes Caddy HTTP access logs from an input stream.
// It is used to prevent the deployment logs from being flooded with routine request information.
func caddyLogFilter(r io.Reader) io.Reader {
	pr, pw := io.Pipe()
	go func() {
		s := bufio.NewScanner(r)
		for s.Scan() {
			if !strings.Contains(s.Text(), `"logger":"http.log.access`) {
				b := append([]byte(s.Text()), '\n')
				pw.Write(b)
			}
		}
		if s.Err() != nil {
			fmt.Println(pw, s.Err())
		}
		pw.Close()
	}()
	return pr
}
