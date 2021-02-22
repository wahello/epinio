package client

import (
	"fmt"

	"github.com/spf13/cobra"
)

var CmdDisable = &cobra.Command{
	Use:           "disable",
	Short:         "disable Carrier features",
	Long:          `disable Carrier features which where enabled with "carrier enable"`,
	Args:          cobra.ExactArgs(0),
	SilenceErrors: true,
	SilenceUsage:  true,
}

// TODO: Implement a flag to also delete provisioned services [TBD]
var CmdDisableGoogle = &cobra.Command{
	Use:           "google",
	Short:         "disable Google cloud services in Carrier",
	Long:          `disable Google cloud services in Carrier which will disable provisioning of those services. Doesn't delete already provisioned services by default.`,
	Args:          cobra.ExactArgs(0),
	RunE:          DisableGoogle,
	SilenceErrors: true,
	SilenceUsage:  true,
}

func init() {
	CmdDisable.AddCommand(CmdDisableGoogle)
}

func DisableGoogle(cmd *cobra.Command, args []string) error {
	fmt.Println("Disabling google things")
	return nil
}
