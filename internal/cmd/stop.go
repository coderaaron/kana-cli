package cmd

import (
	"fmt"
	"os"

	"github.com/ChrisWiegman/kana/internal/site"

	"github.com/spf13/cobra"
)

func newStopCommand(site *site.Site) *cobra.Command {

	cmd := &cobra.Command{
		Use:   "stop",
		Short: "Stops the WordPress development environment.",
		Run: func(cmd *cobra.Command, args []string) {
			runStop(cmd, args, site)
		},
	}

	return cmd

}

func runStop(cmd *cobra.Command, args []string, site *site.Site) {

	err := site.StopWordPress()
	if err != nil {
		fmt.Println(err)
		os.Exit(1)
	}
}
