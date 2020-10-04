package image

import (
	"context"
	"fmt"
	"io"
	"path/filepath"
	"strings"

	"github.com/containers/common/pkg/retry"
	cp "github.com/containers/image/v5/copy"
	"github.com/containers/image/v5/directory"
	"github.com/containers/image/v5/docker"
	"github.com/containers/image/v5/docker/archive"
	dockerarchive "github.com/containers/image/v5/docker/archive"
	ociarchive "github.com/containers/image/v5/oci/archive"
	oci "github.com/containers/image/v5/oci/layout"
	is "github.com/containers/image/v5/storage"
	"github.com/containers/image/v5/transports"
	"github.com/containers/image/v5/transports/alltransports"
	"github.com/containers/image/v5/types"
	"github.com/containers/podman/v2/libpod/events"
	"github.com/containers/podman/v2/pkg/registries"
	"github.com/hashicorp/go-multierror"
	"github.com/opentracing/opentracing-go"
	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"
)

var (
	// DockerArchive is the transport we prepend to an image name
	// when saving to docker-archive
	DockerArchive = dockerarchive.Transport.Name()
	// OCIArchive is the transport we prepend to an image name
	// when saving to oci-archive
	OCIArchive = ociarchive.Transport.Name()
	// DirTransport is the transport for pushing and pulling
	// images to and from a directory
	DirTransport = directory.Transport.Name()
	// DockerTransport is the transport for docker registries
	DockerTransport = docker.Transport.Name()
	// OCIDirTransport is the transport for pushing and pulling
	// images to and from a directory containing an OCI image
	OCIDirTransport = oci.Transport.Name()
	// AtomicTransport is the transport for atomic registries
	AtomicTransport = "atomic"
	// DefaultTransport is a prefix that we apply to an image name
	// NOTE: This is a string prefix, not actually a transport name usable for transports.Get();
	// and because syntaxes of image names are transport-dependent, the prefix is not really interchangeable;
	// each user implicitly assumes the appended string is a Docker-like reference.
	DefaultTransport = DockerTransport + "://"
	// DefaultLocalRegistry is the default local registry for local image operations
	// Remote pulls will still use defined registries
	DefaultLocalRegistry = "localhost"
)

// pullRefPair records a pair of prepared image references to pull.
type pullRefPair struct {
	image  string
	srcRef types.ImageReference
	dstRef types.ImageReference
}

// cleanUpFunc is a function prototype for clean-up functions.
type cleanUpFunc func() error

// pullGoal represents the prepared image references and decided behavior to be executed by imagePull
type pullGoal struct {
	refPairs             []pullRefPair
	pullAllPairs         bool          // Pull all refPairs instead of stopping on first success.
	usedSearchRegistries bool          // refPairs construction has depended on registries.GetRegistries()
	searchedRegistries   []string      // The list of search registries used; set only if usedSearchRegistries
	cleanUpFuncs         []cleanUpFunc // Mainly used to close long-lived objects (e.g., an archive.Reader)
}

// cleanUp invokes all cleanUpFuncs.  Certain resources may not be available
// anymore.  Errors are logged.
func (p *pullGoal) cleanUp() {
	for _, f := range p.cleanUpFuncs {
		if err := f(); err != nil {
			logrus.Error(err.Error())
		}
	}
}

// singlePullRefPairGoal returns a no-frills pull goal for the specified reference pair.
func singlePullRefPairGoal(rp pullRefPair) *pullGoal {
	return &pullGoal{
		refPairs:             []pullRefPair{rp},
		pullAllPairs:         false, // Does not really make a difference.
		usedSearchRegistries: false,
		searchedRegistries:   nil,
	}
}

func (ir *Runtime) getPullRefPair(srcRef types.ImageReference, destName string) (pullRefPair, error) {
	decomposedDest, err := decompose(destName)
	if err == nil && !decomposedDest.hasRegistry {
		// If the image doesn't have a registry, set it as the default repo
		ref, err := decomposedDest.referenceWithRegistry(DefaultLocalRegistry)
		if err != nil {
			return pullRefPair{}, err
		}
		destName = ref.String()
	}

	reference := destName
	if srcRef.DockerReference() != nil {
		reference = srcRef.DockerReference().String()
	}
	destRef, err := is.Transport.ParseStoreReference(ir.store, reference)
	if err != nil {
		return pullRefPair{}, errors.Wrapf(err, "error parsing dest reference name %#v", destName)
	}
	return pullRefPair{
		image:  destName,
		srcRef: srcRef,
		dstRef: destRef,
	}, nil
}

// getSinglePullRefPairGoal calls getPullRefPair with the specified parameters, and returns a single-pair goal for the return value.
func (ir *Runtime) getSinglePullRefPairGoal(srcRef types.ImageReference, destName string) (*pullGoal, error) {
	rp, err := ir.getPullRefPair(srcRef, destName)
	if err != nil {
		return nil, err
	}
	return singlePullRefPairGoal(rp), nil
}

// getPullRefPairsFromDockerArchiveReference returns a slice of pullRefPairs
// for the specified docker reference and the corresponding archive.Reader.
func (ir *Runtime) getPullRefPairsFromDockerArchiveReference(ctx context.Context, reader *archive.Reader, ref types.ImageReference, sc *types.SystemContext) ([]pullRefPair, error) {
	destNames, err := reader.ManifestTagsForReference(ref)
	if err != nil {
		return nil, err
	}

	if len(destNames) == 0 {
		destName, err := getImageDigest(ctx, ref, sc)
		if err != nil {
			return nil, err
		}
		destNames = append(destNames, destName)
	} else {
		for i := range destNames {
			ref, err := NormalizedTag(destNames[i])
			if err != nil {
				return nil, err
			}
			destNames[i] = ref.String()
		}
	}

	refPairs := []pullRefPair{}
	for _, destName := range destNames {
		destRef, err := is.Transport.ParseStoreReference(ir.store, destName)
		if err != nil {
			return nil, errors.Wrapf(err, "error parsing dest reference name %#v", destName)
		}
		pair := pullRefPair{
			image:  destName,
			srcRef: ref,
			dstRef: destRef,
		}
		refPairs = append(refPairs, pair)
	}

	return refPairs, nil
}

// pullGoalFromImageReference returns a pull goal for a single ImageReference, depending on the used transport.
// Note that callers are responsible for invoking (*pullGoal).cleanUp() to clean up possibly open resources.
func (ir *Runtime) pullGoalFromImageReference(ctx context.Context, srcRef types.ImageReference, imgName string, sc *types.SystemContext) (*pullGoal, error) {
	span, _ := opentracing.StartSpanFromContext(ctx, "pullGoalFromImageReference")
	defer span.Finish()

	// supports pulling from docker-archive, oci, and registries
	switch srcRef.Transport().Name() {
	case DockerArchive:
		reader, readerRef, err := archive.NewReaderForReference(sc, srcRef)
		if err != nil {
			return nil, err
		}

		pairs, err := ir.getPullRefPairsFromDockerArchiveReference(ctx, reader, readerRef, sc)
		if err != nil {
			// No need to defer for a single error path.
			if err := reader.Close(); err != nil {
				logrus.Error(err.Error())
			}
			return nil, err
		}

		return &pullGoal{
			pullAllPairs:         true,
			usedSearchRegistries: false,
			refPairs:             pairs,
			searchedRegistries:   nil,
			cleanUpFuncs:         []cleanUpFunc{reader.Close},
		}, nil

	case OCIArchive:
		// imgName such as "/tmp/FOO.tar:" or "/tmp/FOO.tar:domain.com/foo:tag1"
		imgName := strings.TrimSuffix(srcRef.StringWithinTransport(), ":")
		imgNameSli := strings.SplitN(imgName, ":", 2)
		if len(imgNameSli) == 2 {
			// Fixes: https://github.com/containers/podman/issues/7337
			// use the empty name to test the manifest's length at first
			testSrcRef, err := ociarchive.NewReference(imgNameSli[0], "")
			if err != nil {
				return nil, err
			}
			_, err = ociarchive.LoadManifestDescriptor(testSrcRef)
			if err == nil {
				// err is equal nil, this means the manifest's length is 1
				// so could use the name override it
				return ir.getSinglePullRefPairGoal(testSrcRef, imgNameSli[1])
			}
		}

		// retrieve the manifest from index.json to access the image name
		manifest, err := ociarchive.LoadManifestDescriptor(srcRef)
		if err != nil {
			return nil, errors.Wrapf(err, "error loading manifest for %q", srcRef)
		}
		var dest string
		if manifest.Annotations == nil || manifest.Annotations["org.opencontainers.image.ref.name"] == "" {
			// If the input image has no image.ref.name, we need to feed it a dest anyways
			// use the hex of the digest
			dest, err = getImageDigest(ctx, srcRef, sc)
			if err != nil {
				return nil, errors.Wrapf(err, "error getting image digest; image reference not found")
			}
		} else {
			dest = manifest.Annotations["org.opencontainers.image.ref.name"]
		}
		return ir.getSinglePullRefPairGoal(srcRef, dest)

	case DirTransport:
		image := toLocalImageName(srcRef.StringWithinTransport())
		return ir.getSinglePullRefPairGoal(srcRef, image)

	case OCIDirTransport:
		split := strings.SplitN(srcRef.StringWithinTransport(), ":", 2)
		image := toLocalImageName(split[0])
		return ir.getSinglePullRefPairGoal(srcRef, image)

	default:
		return ir.getSinglePullRefPairGoal(srcRef, imgName)
	}
}

// toLocalImageName converts an image name into a 'localhost/' prefixed one
func toLocalImageName(imageName string) string {
	return fmt.Sprintf(
		"%s/%s",
		DefaultLocalRegistry,
		strings.TrimLeft(imageName, "/"),
	)
}

// pullImageFromHeuristicSource pulls an image based on inputName, which is heuristically parsed and may involve configured registries.
// Use pullImageFromReference if the source is known precisely.
func (ir *Runtime) pullImageFromHeuristicSource(ctx context.Context, inputName string, writer io.Writer, authfile, signaturePolicyPath string, signingOptions SigningOptions, dockerOptions *DockerRegistryOptions, retryOptions *retry.RetryOptions, label *string) ([]string, error) {
	span, _ := opentracing.StartSpanFromContext(ctx, "pullImageFromHeuristicSource")
	defer span.Finish()

	var goal *pullGoal
	sc := GetSystemContext(signaturePolicyPath, authfile, false)
	if dockerOptions != nil {
		sc.OSChoice = dockerOptions.OSChoice
		sc.ArchitectureChoice = dockerOptions.ArchitectureChoice
		sc.VariantChoice = dockerOptions.VariantChoice
	}
	if signaturePolicyPath == "" {
		sc.SignaturePolicyPath = ir.SignaturePolicyPath
	}
	sc.BlobInfoCacheDir = filepath.Join(ir.store.GraphRoot(), "cache")
	srcRef, err := alltransports.ParseImageName(inputName)
	if err != nil {
		// We might be pulling with an unqualified image reference in which case
		// we need to make sure that we're not using any other transport.
		srcTransport := alltransports.TransportFromImageName(inputName)
		if srcTransport != nil && srcTransport.Name() != DockerTransport {
			return nil, err
		}
		goal, err = ir.pullGoalFromPossiblyUnqualifiedName(inputName)
		if err != nil {
			return nil, errors.Wrap(err, "error getting default registries to try")
		}
	} else {
		goal, err = ir.pullGoalFromImageReference(ctx, srcRef, inputName, sc)
		if err != nil {
			return nil, errors.Wrapf(err, "error determining pull goal for image %q", inputName)
		}
	}
	defer goal.cleanUp()
	return ir.doPullImage(ctx, sc, *goal, writer, signingOptions, dockerOptions, retryOptions, label)
}

// pullImageFromReference pulls an image from a types.imageReference.
func (ir *Runtime) pullImageFromReference(ctx context.Context, srcRef types.ImageReference, writer io.Writer, authfile, signaturePolicyPath string, signingOptions SigningOptions, dockerOptions *DockerRegistryOptions, retryOptions *retry.RetryOptions) ([]string, error) {
	span, _ := opentracing.StartSpanFromContext(ctx, "pullImageFromReference")
	defer span.Finish()

	sc := GetSystemContext(signaturePolicyPath, authfile, false)
	if dockerOptions != nil {
		sc.OSChoice = dockerOptions.OSChoice
		sc.ArchitectureChoice = dockerOptions.ArchitectureChoice
		sc.VariantChoice = dockerOptions.VariantChoice
	}
	goal, err := ir.pullGoalFromImageReference(ctx, srcRef, transports.ImageName(srcRef), sc)
	if err != nil {
		return nil, errors.Wrapf(err, "error determining pull goal for image %q", transports.ImageName(srcRef))
	}
	defer goal.cleanUp()
	return ir.doPullImage(ctx, sc, *goal, writer, signingOptions, dockerOptions, retryOptions, nil)
}

func cleanErrorMessage(err error) string {
	errMessage := strings.TrimPrefix(errors.Cause(err).Error(), "errors:\n")
	errMessage = strings.Split(errMessage, "\n")[0]
	return fmt.Sprintf("  %s\n", errMessage)
}

// doPullImage is an internal helper interpreting pullGoal. Almost everyone should call one of the callers of doPullImage instead.
func (ir *Runtime) doPullImage(ctx context.Context, sc *types.SystemContext, goal pullGoal, writer io.Writer, signingOptions SigningOptions, dockerOptions *DockerRegistryOptions, retryOptions *retry.RetryOptions, label *string) ([]string, error) {
	span, _ := opentracing.StartSpanFromContext(ctx, "doPullImage")
	defer span.Finish()

	policyContext, err := getPolicyContext(sc)
	if err != nil {
		return nil, err
	}
	defer func() {
		if err := policyContext.Destroy(); err != nil {
			logrus.Errorf("failed to destroy policy context: %q", err)
		}
	}()

	systemRegistriesConfPath := registries.SystemRegistriesConfPath()

	var (
		images     []string
		pullErrors *multierror.Error
	)

	for _, imageInfo := range goal.refPairs {
		copyOptions := getCopyOptions(sc, writer, dockerOptions, nil, signingOptions, "", nil)
		copyOptions.SourceCtx.SystemRegistriesConfPath = systemRegistriesConfPath // FIXME: Set this more globally.  Probably no reason not to have it in every types.SystemContext, and to compute the value just once in one place.
		// Print the following statement only when pulling from a docker or atomic registry
		if writer != nil && (imageInfo.srcRef.Transport().Name() == DockerTransport || imageInfo.srcRef.Transport().Name() == AtomicTransport) {
			if _, err := io.WriteString(writer, fmt.Sprintf("Trying to pull %s...\n", imageInfo.image)); err != nil {
				return nil, err
			}
		}
		// If the label is not nil, check if the label exists and if not, return err
		if label != nil {
			if err := checkRemoteImageForLabel(ctx, *label, imageInfo, sc); err != nil {
				return nil, err
			}
		}
		imageInfo := imageInfo
		if err = retry.RetryIfNecessary(ctx, func() error {
			_, err = cp.Image(ctx, policyContext, imageInfo.dstRef, imageInfo.srcRef, copyOptions)
			return err
		}, retryOptions); err != nil {
			pullErrors = multierror.Append(pullErrors, err)
			logrus.Debugf("Error pulling image ref %s: %v", imageInfo.srcRef.StringWithinTransport(), err)
			if writer != nil {
				_, _ = io.WriteString(writer, cleanErrorMessage(err))
			}
		} else {
			if !goal.pullAllPairs {
				ir.newImageEvent(events.Pull, "")
				return []string{imageInfo.image}, nil
			}
			images = append(images, imageInfo.image)
		}
	}
	// If no image was found, we should handle.  Lets be nicer to the user and see if we can figure out why.
	if len(images) == 0 {
		if goal.usedSearchRegistries && len(goal.searchedRegistries) == 0 {
			return nil, errors.Errorf("image name provided is a short name and no search registries are defined in the registries config file.")
		}
		// If the image passed in was fully-qualified, we will have 1 refpair.  Bc the image is fq'd, we don't need to yap about registries.
		if !goal.usedSearchRegistries {
			if pullErrors != nil && len(pullErrors.Errors) > 0 { // this should always be true
				return nil, pullErrors.Errors[0]
			}
			return nil, errors.Errorf("unable to pull image, or you do not have pull access")
		}
		return nil, errors.Cause(pullErrors)
	}
	if len(images) > 0 {
		ir.newImageEvent(events.Pull, images[0])
	}
	return images, nil
}

// pullGoalFromPossiblyUnqualifiedName looks at inputName and determines the possible
// image references to try pulling in combination with the registries.conf file as well
func (ir *Runtime) pullGoalFromPossiblyUnqualifiedName(inputName string) (*pullGoal, error) {
	decomposedImage, err := decompose(inputName)
	if err != nil {
		return nil, err
	}

	if decomposedImage.hasRegistry {
		srcRef, err := docker.ParseReference("//" + inputName)
		if err != nil {
			return nil, errors.Wrapf(err, "unable to parse '%s'", inputName)
		}
		return ir.getSinglePullRefPairGoal(srcRef, inputName)
	}

	searchRegistries, err := registries.GetRegistries()
	if err != nil {
		return nil, err
	}
	refPairs := make([]pullRefPair, 0, len(searchRegistries))
	for _, registry := range searchRegistries {
		ref, err := decomposedImage.referenceWithRegistry(registry)
		if err != nil {
			return nil, err
		}
		imageName := ref.String()
		srcRef, err := docker.ParseReference("//" + imageName)
		if err != nil {
			return nil, errors.Wrapf(err, "unable to parse '%s'", imageName)
		}
		ps, err := ir.getPullRefPair(srcRef, imageName)
		if err != nil {
			return nil, err
		}
		refPairs = append(refPairs, ps)
	}
	return &pullGoal{
		refPairs:             refPairs,
		pullAllPairs:         false,
		usedSearchRegistries: true,
		searchedRegistries:   searchRegistries,
	}, nil
}

// checkRemoteImageForLabel checks if the remote image has a specific label. if the label exists, we
// return nil, else we return an error
func checkRemoteImageForLabel(ctx context.Context, label string, imageInfo pullRefPair, sc *types.SystemContext) error {
	labelImage, err := imageInfo.srcRef.NewImage(ctx, sc)
	if err != nil {
		return err
	}
	remoteInspect, err := labelImage.Inspect(ctx)
	if err != nil {
		return err
	}
	// Labels are case insensitive; so we iterate instead of simple lookup
	for k := range remoteInspect.Labels {
		if strings.ToLower(label) == strings.ToLower(k) {
			return nil
		}
	}
	return errors.Errorf("%s has no label %s in %q", imageInfo.image, label, remoteInspect.Labels)
}
