package pipeline

import (
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
// to the hub and he database. It returns the host:port address of the
// running contianer so the router can configure Caddy to point at it.
func (p *Pipeline) StartContainer(ctx context.Context, id int64, imageTag string) (string, error) {
	status := data.DEPLOYING
	if err := p.deployment.UpdateDeployment(id, data.DeploymentUpdate{Status: &status}); err != nil {
		return "", fmt.Errorf("failed to update status: %w", err)
	}
	p.hub.PublishStatus(id, status, "")

	p.mu.Lock()
	defer p.mu.Unlock()

	dockerClient, err := client.New(client.FromEnv)
	if err != nil {
		return "", fmt.Errorf("failed to create docker client: %w", err)
	}
	defer dockerClient.Close()

	hostPort, err := getFreePort()
	if err != nil {
		return "", fmt.Errorf("filed to find free port: %w", err)
	}

	imageInspectResult, err := dockerClient.ImageInspect(ctx, imageTag)
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

	// prepare env vars
	deployment, _ := p.deployment.GetDeployment(id)
	var env []string
	if deployment.EnvVars != nil && *deployment.EnvVars != "" {
		lines := strings.Split(*deployment.EnvVars, "\n")
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

	createResp, err := dockerClient.ContainerCreate(ctx, client.ContainerCreateOptions{
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

	_, err = dockerClient.ContainerStart(ctx, createResp.ID, client.ContainerStartOptions{})
	if err != nil {
		return "", fmt.Errorf("failed to start container: %w", err)
	}

	address := net.JoinHostPort(containerName, strings.Split(containerPortStr, "/")[0])

	msg := fmt.Sprintf("waiting for container %s to respond...", address)
	logID, _ := p.deployment.AppendLog(id, "deploy", msg)
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
		logID, _ := p.deployment.AppendLog(id, "deploy", msg)
		p.hub.PublishLog(id, hub.LogMessage{ID: logID, Line: msg})
		return "", fmt.Errorf("container did not respond on port %d", hostPort)
	} else {
		msg := "container is healthy and accepting connections"
		logID, _ := p.deployment.AppendLog(id, "deploy", msg)
		p.hub.PublishLog(id, hub.LogMessage{ID: logID, Line: msg})
	}

	go func() {
		logCtx := context.Background()
		logReader, err := dockerClient.ContainerLogs(logCtx, createResp.ID, client.ContainerLogsOptions{
			ShowStdout: true,
			ShowStderr: true,
			Follow:     true,
			Timestamps: false,
		})
		if err != nil {
			p.deployment.AppendLog(id, "deploy", fmt.Sprintf("failed to attach container logs: %v", err))
			return
		}
		defer logReader.Close()

		// Create an in-memory pipe: pw is the write end, pr is the read end.
		// Anything written to pw can be read from pr.
		pr, pw := io.Pipe()
		// stdcopy.StdCopy runs in its own goroutine because it blocks until
		// logReader is exhausted. It strips the 8-byte Docker headers and
		// writes clean stdout bytes into pw. Stderr chunks go to io.Discard,
		// Go's `/dev/null` (doing nothing with stderr)
		go func() {
			stdcopy.StdCopy(pw, io.Discard, logReader)
			pw.Close()
		}()

		p.streamLogs(id, "deploy", pr)
	}()

	return address, nil
}

func (p *Pipeline) StopContainer(id int64) error {
	URL := ""
	imageTag := ""
	status := data.STOPPED

	dockerClient, err := client.New(client.FromEnv)
	if err != nil {
		return fmt.Errorf("failed to create docker client: %w", err)
	}
	defer dockerClient.Close()

	// Since AutoRemove: true was set in StartContainer,
	// stopping the container will automatically delete it.
	containerName := fmt.Sprintf("appa-%d", id)
	if _, err := dockerClient.ContainerStop(context.Background(), containerName, client.ContainerStopOptions{}); err != nil {
		if !strings.Contains(err.Error(), "No such container") {
			return fmt.Errorf("failed to stop container %s: %w", containerName, err)
		}
	}
	if err := p.RemoveRoute(id); err != nil {
		fmt.Printf("failed to remove caddy route for %d: %v\n", id, err)
	}

	p.deployment.UpdateDeployment(
		id,
		data.DeploymentUpdate{
			URL:      &URL,
			Status:   &status,
			ImageTag: &imageTag,
		},
	)

	msg := "deployment stopped by user"
	logID, _ := p.deployment.AppendLog(id, "system", msg)
	p.hub.PublishLog(id, hub.LogMessage{ID: logID, Line: msg})
	p.hub.PublishStatus(id, status, "")

	return nil
}

func getFreePort() (int, error) {
	// the port number is automatically chosen with 0 as port in address parameter
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return 0, err
	}
	defer ln.Close()
	return ln.Addr().(*net.TCPAddr).Port, nil
}
