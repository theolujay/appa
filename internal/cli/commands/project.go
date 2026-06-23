package commands

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"

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
	cmd.AddCommand(projectLogsCmd())
	cmd.AddCommand(projectStopCmd())
	cmd.AddCommand(projectRestartCmd())
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
	if iCfg.BaseAPIURL == "" {
		return 0, "", fmt.Errorf("server %q has no API URL", pCfg.Target)
	}

	apiURL := iCfg.BaseAPIURL
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
		return fmt.Errorf("project not found: %s", name)
	}
	return config.Edit(config.Project, name)
}
