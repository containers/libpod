package libpodruntime

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/containers/storage"
	"github.com/projectatomic/libpod/libpod"
	"github.com/urfave/cli"
)

// GetRuntime generates a new libpod runtime configured by command line options
func GetRuntime(c *cli.Context) (*libpod.Runtime, error) {
	storageOpts, err := GetDefaultStoreOptions()
	if err != nil {
		return nil, err
	}
	return GetRuntimeWithStorageOpts(c, &storageOpts)
}

func GetRootlessStorageOpts() (storage.StoreOptions, error) {
	var opts storage.StoreOptions

	opts.RunRoot = filepath.Join(libpod.GetRootlessRuntimeDir(), "run")

	dataDir := os.Getenv("XDG_DATA_DIR")
	if dataDir != "" {
		opts.GraphRoot = filepath.Join(dataDir, "containers", "storage")
	} else {
		home := os.Getenv("HOME")
		if home == "" {
			return opts, fmt.Errorf("HOME not specified")
		}
		opts.GraphRoot = filepath.Join(home, ".containers", "storage")
	}
	opts.GraphDriverName = "vfs"
	return opts, nil
}

func GetDefaultStoreOptions() (storage.StoreOptions, error) {
	storageOpts := storage.DefaultStoreOptions
	if os.Getuid() != 0 {
		var err error
		storageOpts, err = GetRootlessStorageOpts()
		if err != nil {
			return storageOpts, err
		}
	}
	return storageOpts, nil
}

// GetRuntime generates a new libpod runtime configured by command line options
func GetRuntimeWithStorageOpts(c *cli.Context, storageOpts *storage.StoreOptions) (*libpod.Runtime, error) {
	options := []libpod.RuntimeOption{}

	if c.GlobalIsSet("root") {
		storageOpts.GraphRoot = c.GlobalString("root")
	}
	if c.GlobalIsSet("runroot") {
		storageOpts.RunRoot = c.GlobalString("runroot")
	}
	if c.GlobalIsSet("storage-driver") {
		storageOpts.GraphDriverName = c.GlobalString("storage-driver")
	}
	if c.GlobalIsSet("storage-opt") {
		storageOpts.GraphDriverOptions = c.GlobalStringSlice("storage-opt")
	}

	options = append(options, libpod.WithStorageConfig(*storageOpts))

	// TODO CLI flags for image config?
	// TODO CLI flag for signature policy?

	if c.GlobalIsSet("runtime") {
		options = append(options, libpod.WithOCIRuntime(c.GlobalString("runtime")))
	}

	if c.GlobalIsSet("conmon") {
		options = append(options, libpod.WithConmonPath(c.GlobalString("conmon")))
	}
	if c.GlobalIsSet("tmpdir") {
		options = append(options, libpod.WithTmpDir(c.GlobalString("tmpdir")))
	}

	if c.GlobalIsSet("cgroup-manager") {
		options = append(options, libpod.WithCgroupManager(c.GlobalString("cgroup-manager")))
	}

	// TODO flag to set libpod static dir?
	// TODO flag to set libpod tmp dir?

	if c.GlobalIsSet("cni-config-dir") {
		options = append(options, libpod.WithCNIConfigDir(c.GlobalString("cni-config-dir")))
	}
	if c.GlobalIsSet("default-mounts-file") {
		options = append(options, libpod.WithDefaultMountsFile(c.GlobalString("default-mounts-file")))
	}
	options = append(options, libpod.WithHooksDir(c.GlobalString("hooks-dir-path"), c.GlobalIsSet("hooks-dir-path")))

	// TODO flag to set CNI plugins dir?

	return libpod.NewRuntime(options...)
}
