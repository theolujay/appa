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
)

func DeployCmd() *cobra.Command {
	var quiet bool
	cmd := &cobra.Command{
		Use:   "deploy <project-name>",
		Short: "Deploy an already initialized project",
		Args:  cobra.ExactArgs(1),
		RunE: func(_ *cobra.Command, args []string) error {
			return deployFunc(args, quiet)
		},
	}
	cmd.Flags().BoolVar(&quiet, "quiet", false, "Suppress rsync progress output")
	return cmd
}

func deployFunc(args []string, quiet bool) error {
	name := args[0]
	if !config.ProjectExists(name) {
		return fmt.Errorf("project %q doesn't exist: use 'appa project init <source>'", name)
	}
	pCfg, err := config.LoadProject(name)
	if err != nil {
		return fmt.Errorf("load project: %w", err)
	}

	if pCfg.Target == "" {
		return fmt.Errorf("target not set: use 'appa project edit %s'", name)
	}

	iCfg, err := config.LoadInstance(pCfg.Target)
	if err != nil {
		return fmt.Errorf("load instance %q: %w", pCfg.Target, err)
	}

	if !iCfg.SetupDone {
		return fmt.Errorf("instance %q has not been set up: run 'appa setup %s' first", pCfg.Target, pCfg.Target)
	}

	if iCfg.BaseAPIURL == "" {
		return fmt.Errorf("instance %q has no API URL: run 'appa setup %s'", pCfg.Target, pCfg.Target)
	}

	info, err := os.Stat(pCfg.Source)
	if err != nil {
		return fmt.Errorf("project source %q: %w", pCfg.Source, err)
	}
	if !info.IsDir() {
		return fmt.Errorf("project source %q is not a directory", pCfg.Source)
	}

	serverDir := config.ServerDirFor(name)
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

	output.Section("Shipping %s to %s", pCfg.Source, ssh.Target(iCfg.SSHUser, iCfg.SSHHost, iCfg.SSHPort))
	var rOut, rErr io.Writer = os.Stdout, os.Stderr
	if quiet {
		rOut = io.Discard
		rErr = io.Discard
	}
	if err := ssh.Rsync(client, src, serverDir, rOut, rErr); err != nil {
		return fmt.Errorf("rsync failed: %w", err)
	}
	output.Success("%s shipped to %s", pCfg.Source, serverDir)

	output.Section("Triggering deployment")
	body := struct {
		Source      string `json:"source"`
		ProjectName string `json:"project_name"`
	}{
		Source:      serverDir,
		ProjectName: name,
	}
	payload, err := json.Marshal(body)
	if err != nil {
		return fmt.Errorf("marshal request: %w", err)
	}

	resp, err := http.Post(iCfg.BaseAPIURL+"/v1/deployments", "application/json", bytes.NewReader(payload))
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
