package main

import (
	"fmt"
	"os"

	"github.com/pkg/errors"
	"github.com/projectatomic/libpod/cmd/podman/libpodruntime"
	"github.com/projectatomic/libpod/libpod"
	"github.com/urfave/cli"
)

var (
	podRmFlags = []cli.Flag{
		cli.BoolFlag{
			Name:  "force, f",
			Usage: "Force removal of a running pod by first stopping all containers, then removing all containers in the pod.  The default is false",
		},
		cli.BoolFlag{
			Name:  "all, a",
			Usage: "Remove all pods",
		},
	}
	podRmDescription = "Remove one or more pods"
	podRmCommand     = cli.Command{
		Name: "rm",
		Usage: fmt.Sprintf(`podman rm will remove one or more pods from the host. The pod name or ID can be used.
                            A pod with running or attached containrs will not be removed.
                            If --force, -f is specified, all containers will be stopped, then removed.`),
		Description:            podRmDescription,
		Flags:                  podRmFlags,
		Action:                 podRmCmd,
		ArgsUsage:              "",
		UseShortOptionHandling: true,
	}
)

// saveCmd saves the image to either docker-archive or oci
func podRmCmd(c *cli.Context) error {
	ctx := getContext()
	if err := validateFlags(c, rmFlags); err != nil {
		return err
	}

	runtime, err := libpodruntime.GetRuntime(c)
	if err != nil {
		return errors.Wrapf(err, "could not get runtime")
	}
	defer runtime.Shutdown(false)

	args := c.Args()

	if len(args) == 0 && !c.Bool("all") {
		return errors.Errorf("specify one or more pods to remove")
	}

	var delPods []*libpod.Pod
	var lastError error
	if c.Bool("all") || c.Bool("a") {
		delPods, err = runtime.Pods()
		if err != nil {
			return errors.Wrapf(err, "unable to get pod list")
		}
	} else {
		for _, i := range args {
			pod, err := runtime.LookupPod(i)
			if err != nil {
				fmt.Fprintln(os.Stderr, err)
				lastError = errors.Wrapf(err, "unable to find pods %s", i)
				continue
			}
			delPods = append(delPods, pod)
		}
	}
	force := c.IsSet("force")

	for _, pod := range delPods {
		err = runtime.RemovePod(ctx, pod, force, force)
		if err != nil {
			if lastError != nil {
				fmt.Fprintln(os.Stderr, lastError)
			}
			lastError = errors.Wrapf(err, "failed to delete pod %v", pod.ID())
		} else {
			fmt.Println(pod.ID())
		}
	}
	return lastError
}
