package commands

import (
	"errors"
	"fmt"

	"github.com/spf13/cobra"
	"github.com/theolujay/appa/internal/cli/config"
	"github.com/theolujay/appa/internal/cli/output"
)

func ProjectCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "project",
		Short: "Manage Appa projects",
	}

	cmd.AddCommand(projectInitCmd())
	cmd.AddCommand(projectEditCmd())
	return cmd
}

func projectInitCmd() *cobra.Command {
	var target string
	var name string

	cmd := &cobra.Command{
		Use:   "init <source>",
		Short: "Create a new project",
		Args:  cobra.ExactArgs(1),
		RunE: func(_ *cobra.Command, args []string) error {
			return projectInitFunc(args, target, name)
		},
	}
	cmd.Flags().StringVarP(&target, "target", "t", "", "Target instance name")
	cmd.Flags().StringVarP(&name, "name", "n", "", "Project name (inferred from source if not specified)")
	return cmd

}

func projectEditCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "edit <name>",
		Short: "Edit project config in $EDITOR",
		Long: `Opens the project config in the system editor for direct TOML editing.

The editor is chosen from $APPA_EDITOR, $EDITOR, or defaults to "vi".
After saving, the file is validated. If invalid, you can re-edit or abort.`,
		Args: cobra.ExactArgs(1),
		RunE: projectEditFunc,
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

	if target != "" && !config.InstanceExists(target) {
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
