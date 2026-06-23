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
Appa servers. It handles host-level configuration, pre-deployment
checks, security hardening, and remote lifecycle management of your server.

Typical setup workflow:
1. Create a new server:  		appa server init <name>
2. Configure connection target:	appa server set-host <name> root@<ip>
3. Validate remote environment:	appa server preflight <name>
4. Hardened stack deployment:  	appa server setup <name>
  		`,
		Version: vcs.Version(),
	}

	rootCmd.AddCommand(commands.ServerCmd())
	rootCmd.AddCommand(commands.ProjectCmd())
	rootCmd.AddCommand(commands.DeployCmd())

	return rootCmd
}
