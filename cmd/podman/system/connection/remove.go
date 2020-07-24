package connection

import (
	"github.com/containers/common/pkg/config"
	"github.com/containers/libpod/v2/cmd/podman/registry"
	"github.com/containers/libpod/v2/cmd/podman/system"
	"github.com/containers/libpod/v2/pkg/domain/entities"
	"github.com/spf13/cobra"
)

var (
	// Skip creating engines since this command will obtain connection information to said engines
	rmCmd = &cobra.Command{
		Use:                   "remove NAME",
		Args:                  cobra.ExactArgs(1),
		Aliases:               []string{"rm"},
		Long:                  `Delete named destination from podman configuration`,
		Short:                 "Delete named destination",
		DisableFlagsInUseLine: true,
		RunE:                  rm,
		Example: `podman system connection remove devl
  podman system connection rm devl`,
	}
)

func init() {
	registry.Commands = append(registry.Commands, registry.CliCommand{
		Mode:    []entities.EngineMode{entities.ABIMode, entities.TunnelMode},
		Command: rmCmd,
		Parent:  system.ConnectionCmd,
	})
}

func rm(_ *cobra.Command, args []string) error {
	cfg, err := config.ReadCustomConfig()
	if err != nil {
		return err
	}

	if cfg.Engine.ServiceDestinations != nil {
		delete(cfg.Engine.ServiceDestinations, args[0])
	}

	if cfg.Engine.ActiveService == args[0] {
		cfg.Engine.ActiveService = ""
	}

	return cfg.Write()
}
