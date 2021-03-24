package cli

import (
	"github.com/pkg/errors"
	"github.com/spf13/cobra"
	"github.com/suse/carrier/deployments"
	"github.com/suse/carrier/internal/cli/clients"
	"github.com/suse/carrier/internal/duration"
	"github.com/suse/carrier/kubernetes"
	"github.com/suse/carrier/termui"
)

var CmdEnable = &cobra.Command{
	Use:           "enable",
	Short:         "enable Carrier features",
	Long:          `enable Carrier features that are not enabled by default`,
	Args:          cobra.ExactArgs(0),
	SilenceErrors: true,
	SilenceUsage:  true,
}

var CmdEnableInCluster = &cobra.Command{
	Use:           "services-incluster",
	Short:         "enable in-cluster services in Carrier",
	Long:          `enable in-cluster services in Carrier which allows provisioning services which run on the same cluster as Carrier. Should be used mostly for development.`,
	Args:          cobra.ExactArgs(0),
	RunE:          EnableInCluster,
	SilenceErrors: true,
	SilenceUsage:  true,
}

var CmdEnableGoogle = &cobra.Command{
	Use:           "services-google",
	Short:         "enable Google Cloud services in Carrier",
	Long:          `enable Google Cloud services in Carrier which allows provisioning those kind of services.`,
	Args:          cobra.ExactArgs(0),
	RunE:          EnableGoogle,
	SilenceErrors: true,
	SilenceUsage:  true,
}

func init() {
	CmdEnableGoogle.Flags().String("service-account-json", "", "the path to the service_account_json for Google Cloud authentication [required]")
	CmdEnableGoogle.MarkFlagRequired("service-account-json")
	CmdEnable.AddCommand(CmdEnableInCluster)
	CmdEnable.AddCommand(CmdEnableGoogle)
}

func EnableInCluster(cmd *cobra.Command, args []string) error {
	return InstallDeployment(
		cmd, &deployments.Minibroker{Timeout: duration.ToDeployment()},
		kubernetes.InstallationOptions{},
		"You can now use in-cluster services")
}

func EnableGoogle(cmd *cobra.Command, args []string) error {
	serviceAccountJSONPath, err := cmd.Flags().GetString("service-account-json")
	if err != nil {
		return err
	}

	return InstallDeployment(
		cmd, &deployments.GoogleServices{Timeout: duration.ToDeployment()},
		kubernetes.InstallationOptions{
			{
				Name:         "service-account-json",
				Description:  "The path to the service account json file used to authenticate with Google Cloud",
				Type:         kubernetes.StringType,
				Default:      "",
				Value:        serviceAccountJSONPath,
				DeploymentID: deployments.GoogleServicesDeploymentID,
			},
		},
		"You can now use Google Cloud services")
}

func InstallDeployment(cmd *cobra.Command, deployment kubernetes.Deployment, opts kubernetes.InstallationOptions, successMessage string) error {
	uiUI := termui.NewUI()
	installClient, installCleanup, err := clients.NewInstallClient(cmd.Flags(), &opts)
	defer func() {
		if installCleanup != nil {
			installCleanup()
		}
	}()

	if err != nil {
		return errors.Wrap(err, "error initializing cli")
	}
	uiUI.Note().Msg(deployment.ID() + " installing...")
	if err := installClient.InstallDeployment(deployment, installClient.Log); err != nil {
		return err
	}
	uiUI.Note().Msg(successMessage)

	return nil
}
