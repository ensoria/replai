package cmd

import "github.com/spf13/cobra"

const version = "0.1.0"

var versionCmd = &cobra.Command{
	Use:   "version",
	Short: "Print the replai version",
	Args:  cobra.NoArgs,
	Run: func(cmd *cobra.Command, args []string) {
		printJSON(&versionOut{OK: true, Version: version})
	},
}

type versionOut struct {
	OK      bool   `json:"ok"`
	Version string `json:"version"`
}
