package system

import (
	"github.com/containers/libpod/v2/cmd/podman/registry"
	"github.com/containers/libpod/v2/cmd/podman/validate"
	"github.com/containers/libpod/v2/pkg/domain/entities"
	"github.com/spf13/cobra"
)

var (
	// Skip creating engines since this command will obtain connection information to said engines
	noOp = func(cmd *cobra.Command, args []string) error {
		return nil
	}

	ConnectionCmd = &cobra.Command{
		Use:                   "connection",
		Short:                 "Manage remote ssh destinations",
		Long:                  `Manage ssh destination information in podman configuration`,
		DisableFlagsInUseLine: true,
		PersistentPreRunE:     noOp,
		RunE:                  validate.SubCommandExists,
		PersistentPostRunE:    noOp,
		TraverseChildren:      false,
	}
)

func init() {
	registry.Commands = append(registry.Commands, registry.CliCommand{
		Mode:    []entities.EngineMode{entities.ABIMode, entities.TunnelMode},
		Command: ConnectionCmd,
		Parent:  systemCmd,
	})
}
