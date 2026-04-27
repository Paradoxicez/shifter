package main

import (
	"os"

	"github.com/spf13/cobra"
)

var rootCmd = &cobra.Command{
	Use:   "shifter",
	Short: "Shifter — self-hosted LoRaWAN water/electricity monitoring",
	Long: `shifter is the self-hosted LoRaWAN water/electricity monitoring backend.

It wraps ChirpStack as the LoRaWAN Network Server and serves the Shifter dashboard
SPA. Subcommands (added in later plans) include serve, migrate, and create-admin.`,
}

func main() {
	if err := rootCmd.Execute(); err != nil {
		os.Exit(1)
	}
}
