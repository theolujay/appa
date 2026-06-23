package commands

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"

	"github.com/spf13/cobra"
	"github.com/theolujay/appa/internal/cli/config"
	"github.com/theolujay/appa/internal/cli/output"
	"github.com/theolujay/appa/internal/cli/ssh"
	"github.com/theolujay/appa/internal/cli/tui"
)

// apiClient is a convenience wrapper for making authenticated API calls
// to the Appa server. It embeds the base URL and provides helper methods.
type apiClient struct {
	baseURL string
	client  *http.Client
}

func newAPIClient(baseURL string) *apiClient {
	return &apiClient{
		baseURL: baseURL,
		client:  &http.Client{},
	}
}

func (c *apiClient) ensureProject(name string) (*int64, error) {
	body := struct {
		Name string `json:"name"`
	}{Name: name}
	payload, err := json.Marshal(body)
	if err != nil {
		return nil, fmt.Errorf("marshal project request: %w", err)
	}

	resp, err := c.client.Post(c.baseURL+"/v1/projects", "application/json", bytes.NewReader(payload))
	if err != nil {
		return nil, fmt.Errorf("create project api call failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusCreated {
		var result struct {
			Project struct {
				ID int64 `json:"id"`
			} `json:"project"`
		}
		if err = json.NewDecoder(resp.Body).Decode(&result); err != nil {
			return nil, fmt.Errorf("decode project response: %w", err)
		}
		return &result.Project.ID, nil
	}

	// If duplicate, look up the existing project
	if resp.StatusCode == http.StatusUnprocessableEntity {
		resp2, err := c.client.Get(c.baseURL + "/v1/projects?name=" + name)
		if err != nil {
			return nil, fmt.Errorf("lookup project api call failed: %w", err)
		}
		defer resp2.Body.Close()

		if resp2.StatusCode != http.StatusOK {
			return nil, fmt.Errorf("lookup project returned status %d", resp2.StatusCode)
		}

		var result struct {
			Projects []struct {
				ID int64 `json:"id"`
			} `json:"projects"`
		}
		if err = json.NewDecoder(resp2.Body).Decode(&result); err != nil {
			return nil, fmt.Errorf("decode project lookup response: %w", err)
		}
		if len(result.Projects) == 0 {
			return nil, fmt.Errorf("project %q not found after creation attempt", name)
		}
		return &result.Projects[0].ID, nil
	}

	return nil, fmt.Errorf("create project returned status %d", resp.StatusCode)
}

func DeployCmd() *cobra.Command {
	var (
		quiet   bool
		verbose bool
	)
	cmd := &cobra.Command{
		Use:   "deploy [project-name]",
		Short: "Deploy an already initialized project",
		Args:  cobra.MaximumNArgs(1),
		RunE: func(_ *cobra.Command, args []string) error {
			name := ""
			if len(args) > 0 {
				name = args[0]
			}
			if name == "" {
				if err := promptProjectName(&name, "deploy"); err != nil {
					return err
				}
			}
			return deployFunc(name, quiet, verbose)
		},
	}
	cmd.Flags().BoolVar(&quiet, "quiet", false, "Suppress rsync progress output")
	cmd.Flags().BoolVarP(&verbose, "verbose", "v", false, "Show detailed output")
	return cmd
}

func deployFunc(name string, quiet, verbose bool) error {
	if !config.ProjectExists(name) {
		output.Error("Project %q not found\n\tUse 'appa project init <source>'", name)
		return nil
	}
	pCfg, err := config.LoadProject(name)
	if err != nil {
		return fmt.Errorf("load project: %w", err)
	}

	if pCfg.Target == "" {
		output.Warn("No target server set for %q\n\tUse 'appa project edit %s'", name, name)
		return nil
	}

	iCfg, err := config.LoadServer(pCfg.Target)
	if err != nil {
		return fmt.Errorf("load server %q: %w", pCfg.Target, err)
	}

	if !iCfg.SetupDone {
		output.Warn("Server %q has not been set up\n\tRun 'appa server setup %s' first", pCfg.Target, pCfg.Target)
		return nil
	}

	if iCfg.APIBaseURL == "" {
		return fmt.Errorf("server %q has no API URL: run 'appa setup %s'", pCfg.Target, pCfg.Target)
	}

	info, err := os.Stat(pCfg.Source)
	if err != nil {
		return fmt.Errorf("project source %q: %w", pCfg.Source, err)
	}
	if !info.IsDir() {
		return fmt.Errorf("project source %q is not a directory", pCfg.Source)
	}

	serverRemoteDir := config.ServerRemoteDirFor(name)
	src, err := config.ParseProjectSource(pCfg.Source)
	if err != nil {
		return fmt.Errorf("unable to parse path %q: %w", pCfg.Source, err)
	}

	skipVerify := iCfg.SSHSkipVerify
	client := ssh.Client{
		User:         iCfg.SSHUser,
		Host:         iCfg.SSHHost,
		Port:         iCfg.SSHPort,
		IdentityFile: iCfg.SSHIdentityFile,
		SkipVerify:   skipVerify,
	}

	label := fmt.Sprintf("Shipping %s to %s", pCfg.Source, ssh.Target(iCfg.SSHUser, iCfg.SSHHost, iCfg.SSHPort))
	var rOut, rErr io.Writer = os.Stdout, os.Stderr
	if quiet || !verbose {
		rOut = io.Discard
		rErr = io.Discard
	}
	if verbose {
		output.Section("%s", label)
		err = ssh.Rsync(client, src, serverRemoteDir, rOut, rErr)
	} else {
		s := tui.StartSpinner(label)
		err = ssh.Rsync(client, src, serverRemoteDir, rOut, rErr)
		s.Stop(err == nil)
	}
	if err != nil {
		return fmt.Errorf("rsync failed: %w", err)
	}
	output.Success("%s shipped to %s", pCfg.Source, serverRemoteDir)

	output.Section("Ensuring project exists on server")
	api := newAPIClient(iCfg.APIBaseURL)
	projectID, err := api.ensureProject(name)
	if err != nil {
		return fmt.Errorf("ensure project: %w", err)
	}

	output.Section("Triggering deployment")
	body := struct {
		Source      string `json:"source"`
		ProjectName string `json:"project_name"`
		ProjectID   *int64 `json:"project_id"`
	}{
		Source:      serverRemoteDir,
		ProjectName: name,
		ProjectID:   projectID,
	}
	payload, err := json.Marshal(body)
	if err != nil {
		return fmt.Errorf("marshal request: %w", err)
	}

	resp, err := http.Post(iCfg.APIBaseURL+"/v1/deployments", "application/json", bytes.NewReader(payload))
	if err != nil {
		return fmt.Errorf("api call failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusCreated {
		return fmt.Errorf("api returned status %d", resp.StatusCode)
	}

	var result struct {
		Deployment struct {
			ID        int64  `json:"id"`
			Status    string `json:"status"`
			CreatedAt string `json:"created_at"`
		} `json:"deployment"`
	}
	if err = json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return fmt.Errorf("decode response: %w", err)
	}

	output.Success("Deployment created:\n")
	h := []string{"ID", "Status", "Created"}
	r := [][]string{
		{
			fmt.Sprintf("%d", result.Deployment.ID),
			result.Deployment.Status,
			result.Deployment.CreatedAt,
		},
	}
	output.PrintTable(h, r, nil)
	return nil
}
