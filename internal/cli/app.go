package cli

import (
	"github.com/spf13/cobra"
	"github.com/theolujay/appa/internal/cli/commands"
)

func NewApp() *cobra.Command {
	root := &cobra.Command{
		Use:   "appa",
		Short: "Appa CLI -- manage your Appa deployment platform",
		Long: `A CLI for managing Appa instances. Create instance profiles,
run preflight checks, provision remote servers via Ansible, and
manage your Appa Stack.`,
	}

	root.AddCommand(commands.InstanceCmd())
	root.AddCommand(commands.PreflightCmd())
	root.AddCommand(commands.SetupCmd())
	root.AddCommand(commands.ApplyCmd())
	root.AddCommand(commands.StatusCmd())
	root.AddCommand(commands.LogsCmd())
	root.AddCommand(commands.RestartCmd())
	root.AddCommand(commands.UpgradeCmd())

	return root
}
