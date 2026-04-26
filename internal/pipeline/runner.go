package pipeline

import (
	"context"
	"fmt"
	"io"
	"net"
	"net/netip"
	"time"

	"github.com/moby/moby/api/pkg/stdcopy"
	"github.com/moby/moby/api/types/container"
	"github.com/moby/moby/api/types/network"
	"github.com/moby/moby/client"
)

// StartContainer starts a container from the given image tag and streams its logs
// to the hub and he database. It returns the host:port address of the
// running contianer so the router can configure Caddy to point at it.
func (p *Pipeline) StartContainer(deploymentID, imageTag string) (string, error) {
	if err := p.store.UpdateDeploymentStatus(deploymentID, "deploying"); err != nil {
		return "", fmt.Errorf("failed to update status: %w", err)
	}

	// Create a Docker client that connects to the daemon via the Unix
	// socket available at /var/run/docker.sock and mount it into the
	// project's API container in compose.yml
	dockerClient, err := client.New(client.FromEnv)
	if err != nil {
		return "", fmt.Errorf("failed to create docker client: %w", err)
	}
	defer dockerClient.Close()

	// Find a free port on the host for this container so the address is known
	// upfront before creating the container
	hostPort, err := getFreePort()
	if err != nil {
		return "", fmt.Errorf("filed to find free port: %w", err)
	}

	ctx := context.Background()

	containerPort, err := network.ParsePort("3000/tcp")
	if err != nil {
		return "", fmt.Errorf("failed to parse container port: %w", err)
	}

	hostConfig := &container.HostConfig{
		// The PortBindings map tells Docker to bind the host port to a select container port.
		PortBindings: network.PortMap{
			containerPort: []network.PortBinding{{
				HostIP:   netip.MustParseAddr("0.0.0.0"),
				HostPort: fmt.Sprintf("%d", hostPort),
			}},
		},
		// AutoRemove means Docker cleans up the container when it stops,
		// so we don't accumulate dead containers on the host over time.
		AutoRemove: true,
	}
	// Create the container. Give it a deterministic name derived from deploymentId
	// for future reference for rollbacks or restarts.
	resp, err := dockerClient.ContainerCreate(ctx, client.ContainerCreateOptions{
		Name:       fmt.Sprintf("appa-%s", deploymentID),
		Config:     &container.Config{Image: imageTag},
		HostConfig: hostConfig,
	})

	if err != nil {
		return "", fmt.Errorf("failed to create container: %w", err)
	}

	// Start the container. At this point the process inside the container
	// is running, but it's not confirmed it's healthy yet.
	_, err = dockerClient.ContainerStart(ctx, resp.ID, client.ContainerStartOptions{})

	if err != nil {
		return "", fmt.Errorf("failed to start container: %w", err)
	}

	// Give some time to start up before declaring it running
	time.Sleep(2 * time.Second)

	// Stream container's logs in the background
	go func() {
		logReader, err := dockerClient.ContainerLogs(ctx, resp.ID, client.ContainerLogsOptions{
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
			// Close the write end when done so pr gets an EOF and
			// streamLogs knows there's nothing more to read.
			pw.Close()
		}()

		// streamLogs reads clean, header-free lines from the read end of
		// the pipe, exactly as it does with the exec.Command pipes in Build.
		p.streamLogs(deploymentID, "deploy", pr)
	}()

	// This is the address the router will use to configure Caddy
	// host.docker.internal allows Caddy (running in its own container)
	// can reach the container running on the host's port binding.
	address := fmt.Sprintf("host.docker.internal:%d", hostPort)
	return address, nil
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
