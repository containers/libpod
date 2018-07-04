// +build !linux

package libpod

import (
	"context"

	spec "github.com/opencontainers/runtime-spec/specs-go"
)

func (c *Container) cleanupCgroups() error {
	return ErrOSNotSupported
}

func (c *Container) mountSHM(shmOptions string) error {
	return ErrNotImplemented
}

func (c *Container) unmountSHM(mount string) error {
	return ErrNotImplemented
}

func (c *Container) prepare() (err error) {
	return ErrNotImplemented
}

func (c *Container) cleanupNetwork() error {
	return ErrNotImplemented
}

func (c *Container) generateSpec(ctx context.Context) (*spec.Spec, error) {
	return nil, ErrNotImplemented
}
