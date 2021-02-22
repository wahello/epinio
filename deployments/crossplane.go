package deployments

import (
	"context"
	"fmt"
	"os"
	"strings"

	"github.com/kyokomi/emoji"
	"github.com/pkg/errors"
	"github.com/suse/carrier/cli/helpers"
	"github.com/suse/carrier/cli/kubernetes"
	"github.com/suse/carrier/cli/paas/ui"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

type Crossplane struct {
	Debug   bool
	Timeout int
}

const (
	CrossplaneDeploymentID = "crossplane"
	crossplaneVersion      = "1.0.0"
	crossplaneChartFile    = "crossplane-1.0.0.tgz"
)

func (k *Crossplane) ID() string {
	return CrossplaneDeploymentID
}

func (k *Crossplane) Backup(c *kubernetes.Cluster, ui *ui.UI, d string) error {
	return nil
}

func (k *Crossplane) Restore(c *kubernetes.Cluster, ui *ui.UI, d string) error {
	return nil
}

func (k Crossplane) Describe() string {
	return emoji.Sprintf(":cloud:Crossplane version: %s\n", crossplaneVersion)
}

// Delete removes Crossplane from kubernetes cluster
func (k Crossplane) Delete(c *kubernetes.Cluster, ui *ui.UI) error {
	ui.Note().KeeplineUnder(1).Msg("Removing Crossplane...")

	existsAndOwned, err := c.NamespaceExistsAndOwned(CrossplaneDeploymentID)
	if err != nil {
		return errors.Wrapf(err, "failed to check if namespace '%s' is owned or not", CrossplaneDeploymentID)
	}
	if !existsAndOwned {
		ui.Exclamation().Msg("Skipping Crossplane because namespace either doesn't exist or not owned by Carrier")
		return nil
	}

	currentdir, err := os.Getwd()
	if err != nil {
		return errors.New("Failed uninstalling Crossplane: " + err.Error())
	}

	message := "Removing helm release " + CrossplaneDeploymentID
	out, err := helpers.WaitForCommandCompletion(ui, message,
		func() (string, error) {
			helmCmd := fmt.Sprintf("helm uninstall '%s' --namespace %s", CrossplaneDeploymentID, CrossplaneDeploymentID)
			return helpers.RunProc(helmCmd, currentdir, k.Debug)
		},
	)
	if err != nil {
		if strings.Contains(out, "release: not found") {
			ui.Exclamation().Msgf("%s helm release not found, skipping.\n", CrossplaneDeploymentID)
		} else {
			return errors.Wrapf(err, "Failed uninstalling helm release %s: %s", CrossplaneDeploymentID, out)
		}
	}

	message = "Deleting Crossplane namespace " + CrossplaneDeploymentID
	_, err = helpers.WaitForCommandCompletion(ui, message,
		func() (string, error) {
			return "", c.DeleteNamespace(CrossplaneDeploymentID)
		},
	)
	if err != nil {
		return errors.Wrapf(err, "Failed deleting namespace %s", CrossplaneDeploymentID)
	}

	ui.Success().Msg("Crossplane removed")

	return nil
}

func (k Crossplane) apply(c *kubernetes.Cluster, ui *ui.UI, options kubernetes.InstallationOptions, upgrade bool) error {
	action := "install"
	if upgrade {
		action = "upgrade"
	}

	currentdir, err := os.Getwd()
	if err != nil {
		return err
	}

	// TODO: Do we need it quarks enabled?
	// if err = createQuarksMonitoredNamespace(c, CrossplaneDeploymentID); err != nil {
	// 	return err
	// }

	tarPath, err := helpers.ExtractFile(crossplaneChartFile)
	if err != nil {
		return errors.New("Failed to extract embedded file: " + tarPath + " - " + err.Error())
	}
	defer os.Remove(tarPath)

	helmCmd := fmt.Sprintf("helm %s %s --create-namespace --namespace %s %s", action, CrossplaneDeploymentID, CrossplaneDeploymentID, tarPath)
	if out, err := helpers.RunProc(helmCmd, currentdir, k.Debug); err != nil {
		return errors.New("Failed installing Crossplane: " + out)
	}

	err = c.LabelNamespace(CrossplaneDeploymentID, kubernetes.CarrierDeploymentLabelKey, kubernetes.CarrierDeploymentLabelValue)
	if err != nil {
		return err
	}
	if err := c.WaitUntilPodBySelectorExist(ui, CrossplaneDeploymentID, "app=crossplane", k.Timeout); err != nil {
		return errors.Wrap(err, "failed waiting Crossplane deployment to come up")
	}
	if err := c.WaitForPodBySelectorRunning(ui, CrossplaneDeploymentID, "app=crossplane", k.Timeout); err != nil {
		return errors.Wrap(err, "failed waiting Crossplane deployment to come be running")
	}
	if err := c.WaitUntilPodBySelectorExist(ui, CrossplaneDeploymentID, "app=crossplane-rbac-manager", k.Timeout); err != nil {
		return errors.Wrap(err, "failed waiting Crossplane rbac manager to come up")
	}
	if err := c.WaitForPodBySelectorRunning(ui, CrossplaneDeploymentID, "app=crossplane-rbac-manager", k.Timeout); err != nil {
		return errors.Wrap(err, "failed waiting Crossplane rbac manager to come be running")
	}

	ui.Success().Msg("Crossplane deployed")

	return nil
}

func (k Crossplane) GetVersion() string {
	return crossplaneVersion
}

func (k Crossplane) Deploy(c *kubernetes.Cluster, ui *ui.UI, options kubernetes.InstallationOptions) error {

	_, err := c.Kubectl.CoreV1().Namespaces().Get(
		context.Background(),
		CrossplaneDeploymentID,
		metav1.GetOptions{},
	)
	if err == nil {
		return errors.New("Namespace " + CrossplaneDeploymentID + " present already")
	}

	ui.Note().KeeplineUnder(1).Msg("Deploying Crossplane...")

	err = k.apply(c, ui, options, false)
	if err != nil {
		return err
	}

	return nil
}

func (k Crossplane) Upgrade(c *kubernetes.Cluster, ui *ui.UI, options kubernetes.InstallationOptions) error {
	_, err := c.Kubectl.CoreV1().Namespaces().Get(
		context.Background(),
		CrossplaneDeploymentID,
		metav1.GetOptions{},
	)
	if err != nil {
		return errors.New("Namespace " + CrossplaneDeploymentID + " not present")
	}

	ui.Note().Msg("Upgrading Crossplane...")

	return k.apply(c, ui, options, true)
}
