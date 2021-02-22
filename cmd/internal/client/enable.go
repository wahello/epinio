package client

import (
	"fmt"

	"github.com/spf13/cobra"
)

var CmdEnable = &cobra.Command{
	Use:           "enable",
	Short:         "enable Carrier features",
	Long:          `enable Carrier features that are not enabled by default`,
	Args:          cobra.ExactArgs(0),
	SilenceErrors: true,
	SilenceUsage:  true,
}

var CmdEnableGoogle = &cobra.Command{
	Use:           "google",
	Short:         "enable Google cloud services in Carrier",
	Long:          `enable Google cloud service in Carrier which allows provisioning and usage of such services within Carrier`,
	Args:          cobra.ExactArgs(0),
	RunE:          EnableGoogle,
	SilenceErrors: true,
	SilenceUsage:  true,
}

func init() {
	CmdEnable.AddCommand(CmdEnableGoogle)
}

func EnableGoogle(cmd *cobra.Command, args []string) error {
	fmt.Println("Installing google things")
	return nil
}
