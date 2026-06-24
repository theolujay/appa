package commands

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"os"
	"strings"

	"github.com/charmbracelet/huh"
	"github.com/spf13/cobra"
	"github.com/theolujay/appa/internal/cli/config"
	"github.com/theolujay/appa/internal/cli/output"
	"github.com/theolujay/appa/internal/cli/tui"
)

func ProjectCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "project",
		Short: "Manage Appa projects",
	}

	cmd.AddCommand(projectInitCmd())
	cmd.AddCommand(projectEditCmd())
	cmd.AddCommand(projectListCmd())
	cmd.AddCommand(projectLogsCmd())
	cmd.AddCommand(projectStopCmd())
	cmd.AddCommand(projectRestartCmd())
	cmd.AddCommand(projectEnvCmd())
	return cmd
}

func projectLogsCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "logs [name]",
		Short: "Stream deployment logs for a project",
		Args:  cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			var name string
			if len(args) > 0 {
				name = args[0]
			}
			if name == "" {
				if err := promptProjectName(&name, "view logs"); err != nil {
					return err
				}
			}
			return projectLogsFunc(cmd, []string{name})
		},
	}
}

func projectStopCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "stop [name]",
		Short: "Stop the latest deployment for a project",
		Args:  cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			var name string
			if len(args) > 0 {
				name = args[0]
			}
			if name == "" {
				if err := promptProjectName(&name, "stop"); err != nil {
					return err
				}
			}
			return projectStopFunc(cmd, []string{name})
		},
	}
}

func projectRestartCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "restart [name]",
		Short: "Restart the latest deployment for a project",
		Args:  cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			var name string
			if len(args) > 0 {
				name = args[0]
			}
			if name == "" {
				if err := promptProjectName(&name, "restart"); err != nil {
					return err
				}
			}
			return projectRestartFunc(cmd, []string{name})
		},
	}
}

func projectListCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "ls",
		Short: "List all project",
		Args:  cobra.NoArgs,
		RunE:  projectListFunc,
	}
}

func promptProjectName(name *string, action string) error {
	cfgs, err := config.ListProjects()
	if err != nil {
		return err
	}
	if len(cfgs) == 0 {
		return fmt.Errorf("no projects found, run 'appa project init' first")
	}
	options := make([]huh.Option[string], len(cfgs))
	for i, cfg := range cfgs {
		options[i] = huh.NewOption(cfg.Name, cfg.Name)
	}
	return huh.NewSelect[string]().
		Title(fmt.Sprintf("Select a project to %s:", action)).
		Options(options...).
		Value(name).
		WithTheme(tui.ThemeAppa()).
		Run()
}

func getLatestDeployment(name string) (int64, string, error) {
	if !config.ProjectExists(name) {
		return 0, "", fmt.Errorf("project %q doesn't exist", name)
	}

	pCfg, err := config.LoadProject(name)
	if err != nil {
		return 0, "", fmt.Errorf("load project: %w", err)
	}
	if pCfg.Target == "" {
		return 0, "", fmt.Errorf("target not set: use 'appa project edit %s'", name)
	}

	iCfg, err := config.LoadServer(pCfg.Target)
	if err != nil {
		return 0, "", fmt.Errorf("load server %q: %w", pCfg.Target, err)
	}
	if !iCfg.SetupDone {
		return 0, "", fmt.Errorf("server %q has not been set up", pCfg.Target)
	}
	if iCfg.APIBaseURL == "" {
		return 0, "", fmt.Errorf("server %q has no API URL", pCfg.Target)
	}

	apiURL := iCfg.APIBaseURL
	url := fmt.Sprintf("%s/v1/deployments?project_name=%s&sort=-id&page_size=1", apiURL, name)
	resp, err := http.Get(url)
	if err != nil {
		return 0, "", fmt.Errorf("api call failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return 0, "", fmt.Errorf("api returned status %d", resp.StatusCode)
	}

	var result struct {
		Deployments []struct {
			ID int64 `json:"id"`
		} `json:"deployments"`
	}
	if err = json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return 0, "", fmt.Errorf("decode response: %w", err)
	}
	if len(result.Deployments) == 0 {
		return 0, "", fmt.Errorf("no deployments found for project %q", name)
	}

	return result.Deployments[0].ID, apiURL, nil
}

func projectLogsFunc(_ *cobra.Command, args []string) error {
	name := args[0]
	deploymentID, apiURL, err := getLatestDeployment(name)
	if err != nil {
		return err
	}
	return tui.Run(tui.NewLogViewer(apiURL, deploymentID))
}

func projectStopFunc(_ *cobra.Command, args []string) error {
	name := args[0]
	deploymentID, apiURL, err := getLatestDeployment(name)
	if err != nil {
		return err
	}

	req, err := http.NewRequest(http.MethodPut, fmt.Sprintf("%s/v1/deployments/%d/stop", apiURL, deploymentID), nil)
	if err != nil {
		return err
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return fmt.Errorf("api call failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusAccepted {
		return fmt.Errorf("api returned status %d", resp.StatusCode)
	}

	output.Success("Project %q stopped (deployment %d)", name, deploymentID)
	return nil
}

func projectRestartFunc(_ *cobra.Command, args []string) error {
	name := args[0]
	deploymentID, apiURL, err := getLatestDeployment(name)
	if err != nil {
		return err
	}

	req, err := http.NewRequest(http.MethodPut, fmt.Sprintf("%s/v1/deployments/%d/restart", apiURL, deploymentID), nil)
	if err != nil {
		return err
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return fmt.Errorf("api call failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusAccepted {
		return fmt.Errorf("api returned status %d", resp.StatusCode)
	}

	output.Success("Project %q restarted (deployment %d)", name, deploymentID)
	return nil
}

func projectInitCmd() *cobra.Command {
	var target string
	var name string

	cmd := &cobra.Command{
		Use:   "init [source]",
		Short: "Create a new project",
		Args:  cobra.MaximumNArgs(1),
		RunE: func(_ *cobra.Command, args []string) error {
			var source string
			if len(args) > 0 {
				source = args[0]
			}
			if source == "" {
			err := huh.NewInput().
				Title("What is the source directory?").
				Placeholder("e.g. . or ./my-app").
				Value(&source).
				WithTheme(tui.ThemeAppa()).
				Run()
				if err != nil {
					return err
				}
			}
			return projectInitFunc([]string{source}, target, name)
		},
	}
	cmd.Flags().StringVarP(&target, "target", "t", "", "Target server name")
	cmd.Flags().StringVarP(&name, "name", "n", "", "Project name (inferred from source if not specified)")
	return cmd

}

func projectEditCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "edit [name]",
		Short: "Edit project config in $EDITOR",
		Long: `Opens the project config in the system editor for direct TOML editing.

The editor is chosen from $APPA_EDITOR, $EDITOR, or defaults to "vi".
After saving, the file is validated. If invalid, you can re-edit or abort.`,
		Args: cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			var name string
			if len(args) > 0 {
				name = args[0]
			}
			if name == "" {
				if err := promptProjectName(&name, "edit"); err != nil {
					return err
				}
			}
			return projectEditFunc(cmd, []string{name})
		},
	}
}

func projectInitFunc(args []string, target, name string) error {
	var errs []error
	source := args[0]
	isNameDerived := false

	source, err := config.AbsPath(source)
	if err != nil {
		errs = append(errs, err)
	}

	if name != "" && !config.ValidName(name) {
		errs = append(errs, fmt.Errorf("invalid name: %s", name))
	} else {
		// ignore error since it would have been caught at config.AbsPath earlier
		name, _ = config.BasePath(source)
		isNameDerived = true
	}

	if config.ProjectExists(name) {
		errs = append(errs, fmt.Errorf("project already exists: %s", name))
	}

	if target != "" && !config.ServerExists(target) {
		errs = append(errs, fmt.Errorf("target not found: %s", target))
	}

	if len(errs) > 0 {
		return errors.Join(errs...)
	}

	cfg := config.DefaultProject(name, source)
	if target != "" {
		cfg.Target = target
	}

	if err := config.SaveProject(cfg); err != nil {
		return fmt.Errorf("save config: %w", err)
	}

	if isNameDerived {
		output.Success("Project name derived from source")
	}
	output.Success("Project %q created", name)
	output.Success("\tNext: appa deploy %s", name)

	return nil
}

func projectEditFunc(_ *cobra.Command, args []string) error {
	name := args[0]
	if !config.ProjectExists(name) {
		output.Error("Project %q not found", name)
		return nil
	}
	return config.Edit(config.Project, name)
}

func projectEnvCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "env",
		Short: "Manage project environment variables",
	}
	cmd.AddCommand(projectEnvSetCmd())
	cmd.AddCommand(projectEnvGetCmd())
	cmd.AddCommand(projectEnvUnsetCmd())
	return cmd
}

func projectEnvSetCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "set [name] KEY=VALUE [KEY=VALUE ...]",
		Short: "Set environment variables for a project",
		RunE: func(_ *cobra.Command, args []string) error {
			var name string
			var pairs []string
			if len(args) > 0 && config.ProjectExists(args[0]) {
				name = args[0]
				pairs = args[1:]
			} else {
				if err := promptProjectName(&name, "set env vars"); err != nil {
					return err
				}
				pairs = args
			}
			if len(pairs) == 0 {
				return fmt.Errorf("at least one KEY=VALUE pair is required")
			}
			return projectEnvSetFunc(name, pairs)
		},
	}
}

func projectEnvGetCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "get [name] [KEY]",
		Short: "Get environment variables for a project",
		RunE: func(_ *cobra.Command, args []string) error {
			var name string
			key := ""
			if len(args) > 0 && config.ProjectExists(args[0]) {
				name = args[0]
				if len(args) > 1 {
					key = args[1]
				}
			} else {
				if err := promptProjectName(&name, "get env vars"); err != nil {
					return err
				}
				if len(args) > 0 {
					key = args[0]
				}
			}
			return projectEnvGetFunc(name, key)
		},
	}
}

func projectEnvUnsetCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "unset [name] KEY [KEY ...]",
		Short: "Unset environment variables for a project",
		RunE: func(_ *cobra.Command, args []string) error {
			var name string
			var keys []string
			if len(args) > 0 && config.ProjectExists(args[0]) {
				name = args[0]
				keys = args[1:]
			} else {
				if err := promptProjectName(&name, "unset env vars"); err != nil {
					return err
				}
				keys = args
			}
			if len(keys) == 0 {
				return fmt.Errorf("at least one KEY is required")
			}
			return projectEnvUnsetFunc(name, keys)
		},
	}
}

func projectEnvSetFunc(name string, pairs []string) error {
	if !config.ProjectExists(name) {
		output.Error("Project %q doesn't exist", name)
		return nil
	}
	pCfg, err := config.LoadProject(name)
	if err != nil {
		return fmt.Errorf("load project: %w", err)
	}
	if pCfg.Target == "" {
		output.Error("Target not set: use 'appa project edit %s'", name)
		return nil
	}
	iCfg, err := config.LoadServer(pCfg.Target)
	if err != nil {
		return fmt.Errorf("load server %q: %w", pCfg.Target, err)
	}
	if iCfg.APIBaseURL == "" {
		output.Error("Server %q has no API URL", pCfg.Target)
		return nil
	}

	envs := make(map[string]string)
	for _, pair := range pairs {
		kv := splitEnvPair(pair)
		if kv == nil {
			output.Error("Invalid env var format: %q (expected KEY=VALUE)", pair)
			return nil
		}
		envs[kv[0]] = kv[1]
	}

	body := struct {
		EnvVars map[string]string `json:"env_vars"`
	}{EnvVars: envs}

	payload, err := json.Marshal(body)
	if err != nil {
		return fmt.Errorf("marshal request: %w", err)
	}

	projectID, err := resolveProjectID(name, iCfg.APIBaseURL)
	if err != nil {
		return err
	}

	url := fmt.Sprintf("%s/v1/projects/%d/env", iCfg.APIBaseURL, projectID)
	req, err := http.NewRequest(http.MethodPost, url, bytes.NewReader(payload))
	if err != nil {
		return fmt.Errorf("create request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return fmt.Errorf("api call failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		output.Error("API returned status %d", resp.StatusCode)
		return nil
	}

	output.Success("Environment variables set for project %q", name)
	return nil
}

func projectEnvGetFunc(name, key string) error {
	if !config.ProjectExists(name) {
		output.Error("Project %q doesn't exist", name)
		return nil
	}
	pCfg, err := config.LoadProject(name)
	if err != nil {
		return fmt.Errorf("load project: %w", err)
	}
	if pCfg.Target == "" {
		output.Error("Target not set: use 'appa project edit %s'", name)
		return nil
	}
	iCfg, err := config.LoadServer(pCfg.Target)
	if err != nil {
		return fmt.Errorf("load server %q: %w", pCfg.Target, err)
	}
	if iCfg.APIBaseURL == "" {
		output.Error("Server %q has no API URL", pCfg.Target)
		return nil
	}

	projectID, err := resolveProjectID(name, iCfg.APIBaseURL)
	if err != nil {
		return err
	}

	url := fmt.Sprintf("%s/v1/projects/%d/env", iCfg.APIBaseURL, projectID)
	resp, err := http.Get(url)
	if err != nil {
		return fmt.Errorf("api call failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		output.Error("API returned status %d", resp.StatusCode)
		return nil
	}

	var result struct {
		EnvVars []struct {
			Key   string `json:"key"`
			Value string `json:"value"`
		} `json:"env_vars"`
	}
	if err = json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return fmt.Errorf("decode response: %w", err)
	}

	if key != "" {
		for _, ev := range result.EnvVars {
			if ev.Key == key {
				output.Success("%s=%s", ev.Key, ev.Value)
				return nil
			}
		}
		output.Error("Env var %q not found for project %q", key, name)
		return nil
	}

	if len(result.EnvVars) == 0 {
		output.Warn("No environment variables set for project %q", name)
		return nil
	}

	header := []string{"Key", "Value"}
	rows := make([][]string, len(result.EnvVars))
	for i, ev := range result.EnvVars {
		rows[i] = []string{ev.Key, ev.Value}
	}
	output.PrintTable(header, rows, nil)
	return nil
}

func projectEnvUnsetFunc(name string, keys []string) error {
	if !config.ProjectExists(name) {
		output.Error("Project %q doesn't exist", name)
		return nil
	}
	pCfg, err := config.LoadProject(name)
	if err != nil {
		return fmt.Errorf("load project: %w", err)
	}
	if pCfg.Target == "" {
		output.Error("Target not set: use 'appa project edit %s'", name)
		return nil
	}
	iCfg, err := config.LoadServer(pCfg.Target)
	if err != nil {
		return fmt.Errorf("load server %q: %w", pCfg.Target, err)
	}
	if iCfg.APIBaseURL == "" {
		output.Error("Server %q has no API URL", pCfg.Target)
		return nil
	}

	projectID, err := resolveProjectID(name, iCfg.APIBaseURL)
	if err != nil {
		return err
	}

	for _, key := range keys {
		url := fmt.Sprintf("%s/v1/projects/%d/env/%s", iCfg.APIBaseURL, projectID, key)
		req, err := http.NewRequest(http.MethodDelete, url, nil)
		if err != nil {
			return fmt.Errorf("create request: %w", err)
		}
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			return fmt.Errorf("api call failed: %w", err)
		}
		resp.Body.Close()
		if resp.StatusCode != http.StatusOK {
			output.Error("Failed to delete env var %q: API returned status %d", key, resp.StatusCode)
			return nil
		}
	}

	output.Success("Environment variables deleted for project %q: %v", name, keys)
	return nil
}

func splitEnvPair(s string) []string {
	for i, c := range s {
		if c == '=' {
			key := s[:i]
			value := s[i+1:]
			if key != "" {
				return []string{key, value}
			}
			return nil
		}
	}
	return nil
}

func resolveProjectID(name, apiBaseURL string) (int64, error) {
	resp, err := http.Get(fmt.Sprintf("%s/v1/projects?name=%s", apiBaseURL, name))
	if err != nil {
		return 0, fmt.Errorf("api call failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return 0, fmt.Errorf("api returned status %d", resp.StatusCode)
	}

	var result struct {
		Projects []struct {
			ID int64 `json:"id"`
		} `json:"projects"`
	}
	if err = json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return 0, fmt.Errorf("decode response: %w", err)
	}
	if len(result.Projects) == 0 {
		return 0, fmt.Errorf("project %q not found", name)
	}
	return result.Projects[0].ID, nil
}

func projectListFunc(_ *cobra.Command, args []string) error {
	cfgs, err := config.ListProjects()
	if err != nil {
		return err
	}
	if len(cfgs) == 0 {
		output.Warn("No project found.")
		output.Success("Create one: appa project init <source>")
	}

	var rows [][]string
	var dimmed []bool
	for _, p := range cfgs {
		source := p.Source
		if home, err := os.UserHomeDir(); err == nil {
			source = "~" + strings.TrimPrefix(p.Source, home)
		}
		target := p.Target
		dim := false
		if target == "" {
			target = "-"
			dim = true
		}
		rows = append(rows, []string{p.Name, source, target})
		dimmed = append(dimmed, dim)
	}
	output.PrintTable([]string{"Name", "Source", "Target"}, rows, dimmed)
	return nil
}
