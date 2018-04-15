package libpod

import (
	"strings"

	is "github.com/containers/image/storage"
	"github.com/pkg/errors"
	"github.com/projectatomic/libpod/libpod/buildah"
	"github.com/projectatomic/libpod/libpod/image"
	"github.com/sirupsen/logrus"
)

// ContainerCommitOptions is a struct used to commit a container to an image
// It uses buildah's CommitOptions as a base. Long-term we might wish to
// add these to the buildah struct once buildah is more integrated with
//libpod
type ContainerCommitOptions struct {
	buildah.CommitOptions
	Pause   bool
	Author  string
	Message string
	Changes []string
}

// Commit commits the changes between a container and its image, creating a new
// image
func (c *Container) Commit(destImage string, options ContainerCommitOptions) (*image.Image, error) {
	if !c.batched {
		c.lock.Lock()
		defer c.lock.Unlock()

		if err := c.syncContainer(); err != nil {
			return nil, err
		}
	}

	if c.state.State == ContainerStateRunning && options.Pause {
		if err := c.runtime.ociRuntime.pauseContainer(c); err != nil {
			return nil, errors.Wrapf(err, "error pausing container %q", c.ID())
		}
		defer func() {
			if err := c.runtime.ociRuntime.unpauseContainer(c); err != nil {
				logrus.Errorf("error unpausing container %q: %v", c.ID(), err)
			}
		}()
	}

	sc := image.GetSystemContext(options.SignaturePolicyPath, "", false)
	builderOptions := buildah.ImportOptions{
		Container:           c.ID(),
		SignaturePolicyPath: options.SignaturePolicyPath,
	}
	commitOptions := buildah.CommitOptions{
		SignaturePolicyPath: options.SignaturePolicyPath,
		ReportWriter:        options.ReportWriter,
		SystemContext:       sc,
	}
	importBuilder, err := buildah.ImportBuilder(c.runtime.store, builderOptions)
	if err != nil {
		return nil, err
	}

	if options.Author != "" {
		importBuilder.SetMaintainer(options.Author)
	}
	if options.Message != "" {
		importBuilder.SetComment(options.Message)
	}

	// Process user changes
	for _, change := range options.Changes {
		splitChange := strings.Split(change, "=")
		switch strings.ToUpper(splitChange[0]) {
		case "CMD":
			importBuilder.SetCmd(splitChange[1:])
		case "ENTRYPOINT":
			importBuilder.SetEntrypoint(splitChange[1:])
		case "ENV":
			importBuilder.SetEnv(splitChange[1], splitChange[2])
		case "EXPOSE":
			importBuilder.SetPort(splitChange[1])
		case "LABEL":
			importBuilder.SetLabel(splitChange[1], splitChange[2])
		case "STOPSIGNAL":
			// No Set StopSignal
		case "USER":
			importBuilder.SetUser(splitChange[1])
		case "VOLUME":
			importBuilder.AddVolume(splitChange[1])
		case "WORKDIR":
			importBuilder.SetWorkDir(splitChange[1])
		}
	}
	imageRef, err := is.Transport.ParseStoreReference(c.runtime.store, destImage)
	if err != nil {
		return nil, err
	}

	if err = importBuilder.Commit(imageRef, commitOptions); err != nil {
		return nil, err
	}
	return c.runtime.imageRuntime.NewFromLocal(imageRef.DockerReference().String())
}
