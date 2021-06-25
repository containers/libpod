package entities

// AutoUpdateOptions are the options for running auto-update.
type AutoUpdateOptions struct {
	// Authfile to use when contacting registries.
	Authfile string
}

// A scanned image during auto-update.
type AutoUpdateImage struct {
	// Digest prior to an update.
	Digest string
	// ID prior to an update.
	ID string
	// All associated names with the image.
	Names []string
	// Digest after an update.
	NewDigest string
	// ID after an update.
	NewID string
	// Indicates whether the image was updated.
	Updated bool
}

// A scanned container during auto-update.
type AutoUpdateContainer struct {
	// ID of the container prior to an update.
	ID string
	// Name of the container prior to an update.
	Name string
	// The image used by the container.
	Image string
	// The systemd unit the container runs ins.
	SystemdUnit string
	// Indicates whether the container was updated.
	Updated bool
}

// AutoUpdateReport contains the results from running auto-update.
type AutoUpdateReport struct {
	// Scanned containers during auto-update.
	Containers []AutoUpdateContainer
	// Scanned images during auto-update.
	Images []AutoUpdateImage
	// Restarted systemd units during auto-update.
	Units []string
}
