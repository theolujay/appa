package pipeline

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"strings"
	"time"

	"github.com/moby/moby/api/pkg/stdcopy"
	"github.com/moby/moby/api/types/container"
	"github.com/moby/moby/client"
	"github.com/theolujay/appa/internal/data"
)

// StartContainer starts a container from the given image tag and streams its logs
// to the hub and the database. It returns the host:port address of the
// running container so the router can configure Caddy to point at it.
func (p *Pipeline) StartContainer(ctx context.Context, id int64) (string, error) {
	d, err := p.deployment.Get(id)
	if err != nil {
		return "", err
	}

	p.mu.Lock()
	defer p.mu.Unlock()

	hPort, err := getPort()
	if err != nil {
		return "", err
	}

	res, err := p.dockerClient.ImageInspect(ctx, *d.ImageTag)
	if err != nil {
		return "", err
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
		return "", err
	}

	_, err = p.dockerClient.ContainerStart(ctx, createResp.ID, client.ContainerStartOptions{})
	if err != nil {
		return "", err
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
		return "", fmt.Errorf("container did not respond on port %d", hPort)
	}

	go func() {
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
			stdcopy.StdCopy(pw, pw, logReader)
			pw.Close()
		}()

		filtered := caddyLogFilter(pr)
		p.streamLogs(id, phaseDeploy, filtered)
	}()

	return addr, nil
}

// StopContainer stops the Docker container, removes the associated Caddy route, and updates the deployment status to STOPPED.
func (p *Pipeline) StopContainer(dc *pipelineCtx) {
	_, err := p.deployment.Get(dc.ID)
	if err != nil {
		switch {
		case errors.Is(err, data.ErrRecordNotFound):
			dc.err = fmt.Errorf("deployment not found")
		default:
			dc.err = err
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
		errMsg := strings.ToLower(err.Error())
		if !strings.Contains(errMsg, "no such container") {
			dc.err = fmt.Errorf("failed to stop container %s: %s", cName, errMsg)
			return
		}
	}

	err = p.router.RemoveRoute(dc.ID)

	url := ""
	status := STOPPED
	imageTag := ""

	dc.status = status
	dc.update = &data.DeploymentUpdate{
		URL:      &url,
		Status:   &status,
		ImageTag: &imageTag,
	}
	dc.err = err
}

// getPort finds and returns an available TCP port by binding to port 0 on localhost.
func getPort() (int, error) {
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
