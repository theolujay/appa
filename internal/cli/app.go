package cli

import (
	"github.com/spf13/cobra"
	"github.com/theolujay/appa/internal/cli/commands"
	"github.com/theolujay/appa/internal/vcs"
)

func NewApp() *cobra.Command {
	rootCmd := &cobra.Command{
		Use:   "appa",
		Short: "Appa CLI -- manage your Appa deployment platform",
		Long: `
			Appa CLI is the control surface for provisioning and operating self-hosted
			Appa Server instances. It handles host-level configuration, pre-deployment
			checks, security hardening, and remote lifecycle management of your server.

			Typical setup workflow:
			1. Create a new instance:  	appa instance init <name>
			2. Configure connection target:	appa instance set-host <name> root@<ip>
			3. Validate remote environment:	appa preflight <name>
			4. Hardened stack deployment:  	appa setup <name>
  		`,
		Version: vcs.Version(),
	}

	rootCmd.AddCommand(commands.InstanceCmd())
	rootCmd.AddCommand(commands.PreflightCmd())
	rootCmd.AddCommand(commands.SetupCmd())
	rootCmd.AddCommand(commands.ApplyCmd())
	rootCmd.AddCommand(commands.StatusCmd())
	rootCmd.AddCommand(commands.LogsCmd())
	rootCmd.AddCommand(commands.RestartCmd())
	rootCmd.AddCommand(commands.UpgradeCmd())

	rootCmd.AddCommand(commands.ProjectCmd())
 	rootCmd.AddCommand(commands.DeployCmd())

	return rootCmd
}
