package volumes

import (
	"github.com/containers/podman/v2/cmd/podman/common"
	"github.com/containers/podman/v2/cmd/podman/registry"
	"github.com/containers/podman/v2/pkg/domain/entities"
	"github.com/spf13/cobra"
)

var (
	volumeExistsDescription = `If the given volume exists, podman volume exists exits with 0, otherwise the exit code will be 1.`
	volumeExistsCommand     = &cobra.Command{
		Use:               "exists VOLUME",
		Short:             "volume exists",
		Long:              volumeExistsDescription,
		RunE:              volumeExists,
		Example:           `podman volume exists myvol`,
		Args:              cobra.ExactArgs(1),
		ValidArgsFunction: common.AutocompleteVolumes,
	}
)

func init() {
	registry.Commands = append(registry.Commands, registry.CliCommand{
		Mode:    []entities.EngineMode{entities.ABIMode, entities.TunnelMode},
		Command: volumeExistsCommand,
		Parent:  volumeCmd,
	})
}

func volumeExists(cmd *cobra.Command, args []string) error {
	response, err := registry.ContainerEngine().VolumeExists(registry.GetContext(), args[0])
	if err != nil {
		return err
	}
	if !response.Value {
		registry.SetExitCode(1)
	}
	return nil
}
