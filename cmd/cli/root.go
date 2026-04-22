package cli

import (
	"cloudstackctl/pkg/cloudstack"
	"cloudstackctl/pkg/handlers"
	"os"

	"github.com/spf13/cobra"
)

// rootCmd represents the base command when called without any subcommands
var rootCmd = &cobra.Command{
	Use:   "cloudstackctl",
	Short: "Kubernetes-style declarative orchestration tool for Apache CloudStack",
	Long: `cloudstackctl is a Kubernetes-style declarative orchestration tool for Apache CloudStack.
It provides a way to manage virtual machines, networking, and storage via YAML,
supporting create, update, delete, health checks, dependency graphs, and drift detection.`,
}

// standalone enables direct CloudStack API mode without DB/controller
var standalone bool

// Execute adds all child commands to the root command and sets flags appropriately
func Execute() {
	err := rootCmd.Execute()
	if err != nil {
		os.Exit(1)
	}
}

func init() {
	// Global flags (CloudStack credentials)
	rootCmd.PersistentFlags().String("cloudstack-endpoint", "", "CloudStack API endpoint (overrides CLOUDSTACK_ENDPOINT)")
	rootCmd.PersistentFlags().String("cloudstack-api-key", "", "CloudStack API key (overrides CLOUDSTACK_API_KEY)")
	rootCmd.PersistentFlags().String("cloudstack-secret-key", "", "CloudStack secret key (overrides CLOUDSTACK_SECRET_KEY)")
	rootCmd.PersistentFlags().StringP("cloudstack-config", "c", "", "path to CloudStack config file")
	rootCmd.PersistentFlags().BoolVarP(&standalone, "standalone", "s", false, "Run in standalone mode (no DB/controller; use CloudStack APIs directly)")
	rootCmd.PersistentFlags().Bool("no-color", false, "Disable color output")
	rootCmd.PersistentFlags().String("color", "auto", "Color output mode: auto, always, never")

	// Configure cloudstack package from CLI flags before running commands
	rootCmd.PersistentPreRun = func(cmd *cobra.Command, args []string) {
		if cfg, _ := cmd.Flags().GetString("cloudstack-config"); cfg != "" {
			cloudstack.SetConfigFile(cfg)
		}
		noColor, _ := cmd.Flags().GetBool("no-color")
		colorFlag, _ := cmd.Flags().GetString("color")
		switch {
		case noColor:
			handlers.SetColorMode("never")
		case colorFlag == "always":
			handlers.SetColorMode("always")
		case colorFlag == "never":
			handlers.SetColorMode("never")
		default:
			handlers.SetColorMode("auto")
		}
	}
}
