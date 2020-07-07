package healthcheck

import (
	"github.com/containers/libpod/v2/cmd/podman/registry"
	"github.com/containers/libpod/v2/cmd/podman/validate"
	"github.com/containers/libpod/v2/pkg/domain/entities"
	"github.com/spf13/cobra"
)

var (
	// Command: healthcheck
	healthCmd = &cobra.Command{
		Use:   "healthcheck",
		Short: "Manage health checks on containers",
		Long:  "Run health checks on containers",
		RunE:  validate.SubCommandExists,
	}
)

func init() {
	registry.Commands = append(registry.Commands, registry.CliCommand{
		Mode:    []entities.EngineMode{entities.ABIMode, entities.TunnelMode},
		Command: healthCmd,
	})
}
