package images

import (
	"fmt"

	"github.com/containers/podman/v2/cmd/podman/common"
	"github.com/containers/podman/v2/cmd/podman/registry"
	"github.com/containers/podman/v2/cmd/podman/utils"
	"github.com/containers/podman/v2/cmd/podman/validate"
	"github.com/containers/podman/v2/pkg/domain/entities"
	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
)

var (
	description = `Image storage increments a mount counter each time an image is mounted.

  When an image is unmounted, the mount counter is decremented. The image's root filesystem is physically unmounted only when the mount counter reaches zero indicating no other processes are using the mount.

  An unmount can be forced with the --force flag.
`
	unmountCommand = &cobra.Command{
		Use:               "unmount [options] IMAGE [IMAGE...]",
		Aliases:           []string{"umount"},
		Short:             "Unmount an image's root filesystem",
		Long:              description,
		RunE:              unmount,
		Args:              validate.ImagesOrAllArgs,
		ValidArgsFunction: common.AutocompleteImages,
		Example: `podman unmount imgID
  podman unmount imgID1 imgID2 imgID3
  podman unmount --all`,
	}
)

var (
	unmountOpts entities.ImageUnmountOptions
)

func unmountFlags(flags *pflag.FlagSet) {
	flags.BoolVarP(&unmountOpts.All, "all", "a", false, "Unmount all of the currently mounted images")
	flags.BoolVarP(&unmountOpts.Force, "force", "f", false, "Force the complete unmount of the specified mounted images")
}

func init() {
	registry.Commands = append(registry.Commands, registry.CliCommand{
		Mode:    []entities.EngineMode{entities.ABIMode},
		Parent:  imageCmd,
		Command: unmountCommand,
	})
	unmountFlags(unmountCommand.Flags())
}

func unmount(cmd *cobra.Command, args []string) error {
	var errs utils.OutputErrors
	reports, err := registry.ImageEngine().Unmount(registry.GetContext(), args, unmountOpts)
	if err != nil {
		return err
	}
	for _, r := range reports {
		if r.Err == nil {
			fmt.Println(r.Id)
		} else {
			errs = append(errs, r.Err)
		}
	}
	return errs.PrintErrors()
}
