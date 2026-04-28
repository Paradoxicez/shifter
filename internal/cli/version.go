package cli

import (
	"encoding/json"
	"fmt"

	"github.com/shifter-io/shifter/internal/version"
	"github.com/spf13/cobra"
)

var versionJSONOutput bool

var versionCmd = &cobra.Command{
	Use:   "version",
	Short: "Print the binary version",
	RunE: func(cmd *cobra.Command, _ []string) error {
		info := version.Info()
		if versionJSONOutput {
			return json.NewEncoder(cmd.OutOrStdout()).Encode(info)
		}
		fmt.Fprintf(cmd.OutOrStdout(), "shifter %s\ncommit: %s\nbuilt: %s\n",
			info.Version, info.Commit, info.BuildTime)
		return nil
	},
}

func init() {
	versionCmd.Flags().BoolVar(&versionJSONOutput, "json", false, "Emit JSON output")
}
