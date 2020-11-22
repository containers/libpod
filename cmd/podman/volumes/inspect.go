package volumes

import (
	"github.com/containers/podman/v2/cmd/podman/common"
	"github.com/containers/podman/v2/cmd/podman/inspect"
	"github.com/containers/podman/v2/cmd/podman/registry"
	"github.com/containers/podman/v2/cmd/podman/validate"
	"github.com/containers/podman/v2/pkg/domain/entities"
	"github.com/spf13/cobra"
)

var (
	volumeInspectDescription = `Display detailed information on one or more volumes.

  Use a Go template to change the format from JSON.`
	inspectCommand = &cobra.Command{
		Use:               "inspect [options] VOLUME [VOLUME...]",
		Short:             "Display detailed information on one or more volumes",
		Long:              volumeInspectDescription,
		RunE:              volumeInspect,
		Args:              validate.VolumesOrAllArgs,
		ValidArgsFunction: common.AutocompleteVolumes,
		Example: `podman volume inspect myvol
  podman volume inspect --all
  podman volume inspect --format "{{.Driver}} {{.Scope}}" myvol`,
	}
)

var (
	inspectOpts *entities.InspectOptions
)

func init() {
	registry.Commands = append(registry.Commands, registry.CliCommand{
		Mode:    []entities.EngineMode{entities.ABIMode, entities.TunnelMode},
		Command: inspectCommand,
		Parent:  volumeCmd,
	})
	inspectOpts = new(entities.InspectOptions)
	flags := inspectCommand.Flags()
	flags.BoolVarP(&inspectOpts.All, "all", "a", false, "Inspect all volumes")

	formatFlagName := "format"
	flags.StringVarP(&inspectOpts.Format, formatFlagName, "f", "json", "Format volume output using Go template")
	_ = inspectCommand.RegisterFlagCompletionFunc(formatFlagName, common.AutocompleteJSONFormat)
}

func volumeInspect(cmd *cobra.Command, args []string) error {
	inspectOpts.Type = inspect.VolumeType
	return inspect.Inspect(args, *inspectOpts)
}
