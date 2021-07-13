package main

import (
	"fmt"

	"github.com/containers/common/pkg/auth"
	"github.com/containers/common/pkg/completion"
	"github.com/containers/podman/v3/cmd/podman/registry"
	"github.com/containers/podman/v3/pkg/domain/entities"
	"github.com/containers/podman/v3/pkg/errorhandling"
	"github.com/pkg/errors"
	"github.com/spf13/cobra"
)

var (
	autoUpdateOptions     = entities.AutoUpdateOptions{}
	autoUpdateDescription = `Auto update containers according to their auto-update policy.

  Auto-update policies are specified with the "io.containers.autoupdate" label.
  Containers are expected to run in systemd units created with "podman-generate-systemd --new",
  or similar units that create new containers in order to run the updated images.
  Please refer to the podman-auto-update(1) man page for details.`
	autoUpdateCommand = &cobra.Command{
		Annotations:       map[string]string{registry.EngineMode: registry.ABIMode},
		Use:               "auto-update [options]",
		Short:             "Auto update containers according to their auto-update policy",
		Long:              autoUpdateDescription,
		RunE:              autoUpdate,
		ValidArgsFunction: completion.AutocompleteNone,
		Example: `podman auto-update
  podman auto-update --authfile ~/authfile.json`,
	}
)

func init() {
	registry.Commands = append(registry.Commands, registry.CliCommand{
		Command: autoUpdateCommand,
	})

	flags := autoUpdateCommand.Flags()

	authfileFlagName := "authfile"
	flags.StringVar(&autoUpdateOptions.Authfile, authfileFlagName, auth.GetDefaultAuthFile(), "Path to the authentication file. Use REGISTRY_AUTH_FILE environment variable to override")
	_ = autoUpdateCommand.RegisterFlagCompletionFunc(authfileFlagName, completion.AutocompleteDefault)
}

func autoUpdate(cmd *cobra.Command, args []string) error {
	if len(args) > 0 {
		// Backwards compat. System tests expect this error string.
		return errors.Errorf("`%s` takes no arguments", cmd.CommandPath())
	}

	report, failures := registry.ContainerEngine().AutoUpdate(registry.GetContext(), autoUpdateOptions)
	if report == nil {
		return errorhandling.JoinErrors(failures)
	}

	if len(report.Containers) > 0 {
		fmt.Println("Scanned the following containers:")
		for _, c := range report.Containers {
			fmt.Printf(" * ID: %s, name: %s, image: %s, unit: %s, updated: %v\n", c.ID[:12], c.Name, c.Image, c.SystemdUnit, c.Updated)
		}
	}

	if len(report.Images) > 0 {
		fmt.Println("Scanned the following images:")
		for _, i := range report.Images {
			id := i.ID[:12]
			if i.Updated {
				id = i.NewID[:12]
			}
			fmt.Printf(" * ID: %s, tags: %v, updated: %v\n", id, i.Names, i.Updated)
		}
	}

	if len(report.Units) > 0 {
		fmt.Println("Restarted the following systemd units:")
		for _, u := range report.Units {
			fmt.Printf(" * %s\n", u)
		}
	}

	return errorhandling.JoinErrors(failures)
}
