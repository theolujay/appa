package pipeline

import (
	"context"
	"fmt"
	"io"
	"net"
	"net/netip"
	"strings"
	"time"

	"github.com/moby/moby/api/pkg/stdcopy"
	"github.com/moby/moby/api/types/container"
	"github.com/moby/moby/api/types/network"
	"github.com/moby/moby/client"
	"github.com/theolujay/appa/internal/hub"
	"github.com/theolujay/appa/internal/store"
)

// StartContainer starts a container from the given image tag and streams its logs
// to the hub and he database. It returns the host:port address of the
// running contianer so the router can configure Caddy to point at it.
func (p *Pipeline) StartContainer(ctx context.Context, deploymentID, imageTag string) (string, error) {
	status := store.DEPLOYING
	if err := p.store.UpdateDeployment(deploymentID, store.DeploymentUpdate{Status: &status}); err != nil {
		return "", fmt.Errorf("failed to update status: %w", err)
	}
	p.hub.PublishStatus(deploymentID, status, "")

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

	containerPort, err := network.ParsePort(containerPortStr)
	if err != nil {
		return "", fmt.Errorf("failed to parse container port: %w", err)
	}

	// Prepare environment variables
	deployment, _ := p.store.GetDeployment(deploymentID)
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

	hostConfig := &container.HostConfig{
		PortBindings: network.PortMap{
			containerPort: []network.PortBinding{{
				HostIP:   netip.MustParseAddr("0.0.0.0"),
				HostPort: fmt.Sprintf("%d", hostPort),
			}},
		},
		AutoRemove: false,
	}

	createResp, err := dockerClient.ContainerCreate(ctx, client.ContainerCreateOptions{
		Name: fmt.Sprintf("appa-%s", deploymentID),
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
		// p.mu.Unlock()
		return "", fmt.Errorf("failed to start container: %w", err)
	}
	// p.mu.Unlock()

	msg := fmt.Sprintf("waiting for container to respond on port %d...", hostPort)
	id, _ := p.store.AppendLog(deploymentID, "deploy", msg)
	p.hub.PublishLog(deploymentID, hub.LogMessage{ID: id, Line: msg})

	address := fmt.Sprintf("host.docker.internal:%d", hostPort)

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
		id, _ := p.store.AppendLog(deploymentID, "deploy", msg)
		p.hub.PublishLog(deploymentID, hub.LogMessage{ID: id, Line: msg})
		return "", fmt.Errorf("container did not respond on port %d", hostPort)
	} else {
		msg := "container is healthy and accepting connections"
		id, _ := p.store.AppendLog(deploymentID, "deploy", msg)
		p.hub.PublishLog(deploymentID, hub.LogMessage{ID: id, Line: msg})
	}

	go func() {
		logCtx := context.Background()
		logReader, err := dockerClient.ContainerLogs(logCtx, createResp.ID, client.ContainerLogsOptions{
			ShowStdout: true,
			ShowStderr: true,
			Follow:     true, // keep streaming
			Timestamps: false,
		})
		if err != nil {
			p.store.AppendLog(deploymentID, "deploy", fmt.Sprintf("failed to attach container logs: %v", err))
			return
		}
		defer logReader.Close()

		// Create an in-memory pip: pw is the write end, pr is the read end.
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

		p.streamLogs(deploymentID, "deploy", pr)
	}()

	return address, nil
}

func (p *Pipeline) StopContainer(deploymentID string) error {
	URL := ""
	imageTag := ""
	status := store.STOPPED

	dockerClient, err := client.New(client.FromEnv)
	if err != nil {
		return fmt.Errorf("failed to create docker client: %w", err)
	}
	defer dockerClient.Close()

	// Since AutoRemove: true was set in StartContainer,
	// stopping the container will automatically delete it.
	containerName := fmt.Sprintf("appa-%s", deploymentID)
	if _, err := dockerClient.ContainerStop(context.Background(), containerName, client.ContainerStopOptions{}); err != nil {
		if !strings.Contains(err.Error(), "No such container") {
			return fmt.Errorf("failed to stop container %s: %w", containerName, err)
		}
	}
	if err := p.RemoveRoute(deploymentID); err != nil {
		fmt.Printf("failed to remove caddy route for %s: %v\n", deploymentID, err)
	}

	p.store.UpdateDeployment(
		deploymentID,
		store.DeploymentUpdate{
			URL:      &URL,
			Status:   &status,
			ImageTag: &imageTag,
		},
	)

	msg := "deployment stopped by user"
	id, _ := p.store.AppendLog(deploymentID, "system", msg)
	p.hub.PublishLog(deploymentID, hub.LogMessage{ID: id, Line: msg})
	p.hub.PublishStatus(deploymentID, status, "")

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
