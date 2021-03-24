package cli

import (
	"github.com/pkg/errors"
	"github.com/spf13/cobra"
	"github.com/suse/carrier/internal/cli/clients"
)

func init() {
	CmdServiceCreate.Flags().Bool("dont-wait", false, "Return immediately, without waiting for the service to be provisioned")
	CmdServiceDelete.Flags().Bool("unbind", false, "Unbind from applications before deleting")
	CmdService.AddCommand(CmdServiceShow)
	CmdService.AddCommand(CmdServiceCreate)
	CmdService.AddCommand(CmdServiceCreateCustom)
	CmdService.AddCommand(CmdServiceDelete)
	CmdService.AddCommand(CmdServiceBind)
	CmdService.AddCommand(CmdServiceUnbind)
	CmdService.AddCommand(CmdServiceListClasses)
	CmdService.AddCommand(CmdServiceListPlans)
	CmdService.AddCommand(CmdServiceList)
}

// CmdService implements the carrier service command
var CmdService = &cobra.Command{
	Use:           "service",
	Aliases:       []string{"services"},
	Short:         "Carrier service features",
	Long:          `Handle service features with Carrier`,
	Args:          cobra.ExactArgs(0),
	SilenceErrors: true,
	SilenceUsage:  true,
}

// CmdServiceShow implements the carrier service show command
var CmdServiceShow = &cobra.Command{
	Use:           "show NAME",
	Short:         "Service information",
	Long:          `Show detailed information of the named service.`,
	Args:          cobra.ExactArgs(1),
	RunE:          ServiceShow,
	SilenceErrors: true,
	SilenceUsage:  true,
	ValidArgsFunction: func(cmd *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
		if len(args) != 0 {
			return nil, cobra.ShellCompDirectiveNoFileComp
		}

		app, cleanup, err := clients.NewCarrierClient(cmd.Flags())
		defer func() {
			if cleanup != nil {
				cleanup()
			}
		}()

		if err != nil {
			return nil, cobra.ShellCompDirectiveNoFileComp
		}

		matches := app.ServiceMatching(toComplete)

		return matches, cobra.ShellCompDirectiveNoFileComp
	},
}

// CmdServiceCreate implements the carrier service create command
var CmdServiceCreate = &cobra.Command{
	Use:   "create NAME CLASS PLAN ?(KEY VALUE)...?",
	Short: "Create a service",
	Long:  `Create service by name, class, plan, and optional key/value dictionary.`,
	Args: func(cmd *cobra.Command, args []string) error {
		if len(args) < 3 {
			return errors.New("Not enough arguments, expected name, class, plan, key, and value")
		}
		if len(args)%2 == 0 {
			return errors.New("Last Key has no value")
		}
		return nil
	},
	RunE:          ServiceCreate,
	SilenceErrors: true,
	SilenceUsage:  true,
	ValidArgsFunction: func(cmd *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
		if len(args) > 2 {
			return nil, cobra.ShellCompDirectiveNoFileComp
		}

		if len(args) == 0 {
			// #args == 0: service name. new. nothing to match
			return nil, cobra.ShellCompDirectiveNoFileComp
		}

		app, cleanup, err := clients.NewCarrierClient(cmd.Flags())
		defer func() {
			if cleanup != nil {
				cleanup()
			}
		}()

		if err != nil {
			return nil, cobra.ShellCompDirectiveNoFileComp
		}

		if len(args) == 2 {
			// #args == 2: service plan name.
			matches := app.ServicePlanMatching(args[1], toComplete)
			return matches, cobra.ShellCompDirectiveNoFileComp
		}

		// #args == 1: service class name.
		matches := app.ServiceClassMatching(toComplete)
		return matches, cobra.ShellCompDirectiveNoFileComp
	},
}

// CmdServiceCreateCustom implements the carrier service create-custom command
var CmdServiceCreateCustom = &cobra.Command{
	Use:   "create-custom NAME (KEY VALUE)...",
	Short: "Create a custom service",
	Long:  `Create custom service by name and key/value dictionary.`,
	Args: func(cmd *cobra.Command, args []string) error {
		if len(args) < 3 {
			return errors.New("Not enough arguments, expected name, key, and value")
		}
		if len(args)%2 == 0 {
			return errors.New("Last Key has no value")
		}
		return nil
	},
	RunE:          ServiceCreateCustom,
	SilenceErrors: true,
	SilenceUsage:  true,
}

// CmdServiceDelete implements the carrier service delete command
var CmdServiceDelete = &cobra.Command{
	Use:           "delete NAME",
	Short:         "Delete a service",
	Long:          `Delete service by name.`,
	Args:          cobra.ExactArgs(1),
	RunE:          ServiceDelete,
	SilenceErrors: true,
	SilenceUsage:  true,
	ValidArgsFunction: func(cmd *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
		if len(args) != 0 {
			return nil, cobra.ShellCompDirectiveNoFileComp
		}

		app, cleanup, err := clients.NewCarrierClient(cmd.Flags())
		defer func() {
			if cleanup != nil {
				cleanup()
			}
		}()

		if err != nil {
			return nil, cobra.ShellCompDirectiveNoFileComp
		}

		matches := app.ServiceMatching(toComplete)

		return matches, cobra.ShellCompDirectiveNoFileComp
	},
}

// CmdServiceBind implements the carrier service bind command
var CmdServiceBind = &cobra.Command{
	Use:           "bind NAME APP",
	Short:         "Bind a service to an application",
	Long:          `Bind service by name, to named application.`,
	Args:          cobra.ExactArgs(2),
	RunE:          ServiceBind,
	SilenceErrors: true,
	SilenceUsage:  true,
	ValidArgsFunction: func(cmd *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
		if len(args) > 1 {
			return nil, cobra.ShellCompDirectiveNoFileComp
		}

		app, cleanup, err := clients.NewCarrierClient(cmd.Flags())
		defer func() {
			if cleanup != nil {
				cleanup()
			}
		}()

		if err != nil {
			return nil, cobra.ShellCompDirectiveNoFileComp
		}

		if len(args) == 1 {
			// #args == 1: app name.
			matches := app.AppsMatching(toComplete)
			return matches, cobra.ShellCompDirectiveNoFileComp
		}

		// #args == 0: service name.

		matches := app.ServiceMatching(toComplete)

		return matches, cobra.ShellCompDirectiveNoFileComp
	},
}

// CmdServiceUnbind implements the carrier service unbind command
var CmdServiceUnbind = &cobra.Command{
	Use:           "unbind NAME APP",
	Short:         "Unbind service from an application",
	Long:          `Unbind service by name, from named application.`,
	Args:          cobra.ExactArgs(2),
	RunE:          ServiceUnbind,
	SilenceErrors: true,
	SilenceUsage:  true,
	ValidArgsFunction: func(cmd *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
		if len(args) > 1 {
			return nil, cobra.ShellCompDirectiveNoFileComp
		}

		app, cleanup, err := clients.NewCarrierClient(cmd.Flags())
		defer func() {
			if cleanup != nil {
				cleanup()
			}
		}()

		if err != nil {
			return nil, cobra.ShellCompDirectiveNoFileComp
		}

		if len(args) == 1 {
			// #args == 1: app name.
			matches := app.AppsMatching(toComplete)
			return matches, cobra.ShellCompDirectiveNoFileComp
		}

		// #args == 0: service name.
		matches := app.ServiceMatching(toComplete)

		return matches, cobra.ShellCompDirectiveNoFileComp
	},
}

// CmdServiceListClasses implements the carrier service classes command
var CmdServiceListClasses = &cobra.Command{
	Use:           "list-classes",
	Short:         "Lists the available service classes",
	RunE:          ServiceListClasses,
	SilenceErrors: true,
	SilenceUsage:  true,
}

// CmdServiceListPlans implements the carrier service plans command
var CmdServiceListPlans = &cobra.Command{
	Use:           "list-plans CLASS",
	Short:         "Lists all plans provided by the named service class",
	Args:          cobra.ExactArgs(1),
	RunE:          ServiceListPlans,
	SilenceErrors: true,
	SilenceUsage:  true,
	ValidArgsFunction: func(cmd *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
		if len(args) != 0 {
			return nil, cobra.ShellCompDirectiveNoFileComp
		}

		app, cleanup, err := clients.NewCarrierClient(cmd.Flags())
		defer func() {
			if cleanup != nil {
				cleanup()
			}
		}()

		if err != nil {
			return nil, cobra.ShellCompDirectiveNoFileComp
		}

		matches := app.ServiceClassMatching(toComplete)

		return matches, cobra.ShellCompDirectiveNoFileComp
	},
}

// CmdServiceList implements the carrier service list command
var CmdServiceList = &cobra.Command{
	Use:           "list",
	Short:         "Lists all services",
	RunE:          ServiceList,
	SilenceErrors: true,
	SilenceUsage:  true,
}

// ServiceShow implements the carrier service show command
func ServiceShow(cmd *cobra.Command, args []string) error {
	client, cleanup, err := clients.NewCarrierClient(cmd.Flags())
	defer func() {
		if cleanup != nil {
			cleanup()
		}
	}()

	if err != nil {
		return errors.Wrap(err, "error initializing cli")
	}

	err = client.ServiceDetails(args[0])
	if err != nil {
		return errors.Wrap(err, "error retrieving service")
	}

	return nil
}

// ServiceCreate implements the carrier service create command
func ServiceCreate(cmd *cobra.Command, args []string) error {
	client, cleanup, err := clients.NewCarrierClient(cmd.Flags())
	defer func() {
		if cleanup != nil {
			cleanup()
		}
	}()

	if err != nil {
		return errors.Wrap(err, "error initializing cli")
	}

	dw, err := cmd.Flags().GetBool("dont-wait")
	if err != nil {
		return err
	}
	waitforProvision := !dw

	err = client.CreateService(args[0], args[1], args[2], args[3:], waitforProvision)
	if err != nil {
		return errors.Wrap(err, "error creating service")
	}

	return nil
}

// ServiceCreateCustom implements the carrier service create-custom command
func ServiceCreateCustom(cmd *cobra.Command, args []string) error {
	client, cleanup, err := clients.NewCarrierClient(cmd.Flags())
	defer func() {
		if cleanup != nil {
			cleanup()
		}
	}()

	if err != nil {
		return errors.Wrap(err, "error initializing cli")
	}

	err = client.CreateCustomService(args[0], args[1:])
	if err != nil {
		return errors.Wrap(err, "error creating service")
	}

	return nil
}

// ServiceDelete implements the carrier service delete command
func ServiceDelete(cmd *cobra.Command, args []string) error {
	unbind, err := cmd.Flags().GetBool("unbind")
	if err != nil {
		return err
	}

	client, cleanup, err := clients.NewCarrierClient(cmd.Flags())
	defer func() {
		if cleanup != nil {
			cleanup()
		}
	}()

	if err != nil {
		return errors.Wrap(err, "error initializing cli")
	}

	err = client.DeleteService(args[0], unbind)
	if err != nil {
		return errors.Wrap(err, "error deleting service")
	}

	return nil
}

// ServiceBind implements the carrier service bind command
func ServiceBind(cmd *cobra.Command, args []string) error {
	client, cleanup, err := clients.NewCarrierClient(cmd.Flags())
	defer func() {
		if cleanup != nil {
			cleanup()
		}
	}()

	if err != nil {
		return errors.Wrap(err, "error initializing cli")
	}

	err = client.BindService(args[0], args[1])
	if err != nil {
		return errors.Wrap(err, "error binding service")
	}

	return nil
}

// ServiceUnbind implements the carrier service unbind command
func ServiceUnbind(cmd *cobra.Command, args []string) error {
	client, cleanup, err := clients.NewCarrierClient(cmd.Flags())
	defer func() {
		if cleanup != nil {
			cleanup()
		}
	}()

	if err != nil {
		return errors.Wrap(err, "error initializing cli")
	}

	err = client.UnbindService(args[0], args[1])
	if err != nil {
		return errors.Wrap(err, "error unbinding service")
	}

	return nil
}

// ServiceListClasses implements the carrier service list-classes command
func ServiceListClasses(cmd *cobra.Command, args []string) error {
	client, cleanup, err := clients.NewCarrierClient(cmd.Flags())
	defer func() {
		if cleanup != nil {
			cleanup()
		}
	}()

	if err != nil {
		return errors.Wrap(err, "error initializing cli")
	}

	err = client.ServiceClasses()
	if err != nil {
		return errors.Wrap(err, "error listing service classes")
	}

	return nil
}

// ServiceListPlans implements the carrier service list-plans command
func ServiceListPlans(cmd *cobra.Command, args []string) error {
	client, cleanup, err := clients.NewCarrierClient(cmd.Flags())
	defer func() {
		if cleanup != nil {
			cleanup()
		}
	}()

	if err != nil {
		return errors.Wrap(err, "error initializing cli")
	}

	err = client.ServicePlans(args[0])
	if err != nil {
		return errors.Wrap(err, "error listing plan")
	}

	return nil
}

// ServiceList implements the carrier service list command
func ServiceList(cmd *cobra.Command, args []string) error {
	client, cleanup, err := clients.NewCarrierClient(cmd.Flags())
	defer func() {
		if cleanup != nil {
			cleanup()
		}
	}()

	if err != nil {
		return errors.Wrap(err, "error initializing cli")
	}

	err = client.Services()
	if err != nil {
		return errors.Wrap(err, "error listing services")
	}

	return nil
}
