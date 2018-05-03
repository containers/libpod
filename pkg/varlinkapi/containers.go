package varlinkapi

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"syscall"

	"github.com/pkg/errors"
	"github.com/projectatomic/libpod/cmd/podman/batchcontainer"
	"github.com/projectatomic/libpod/cmd/podman/libpodruntime"
	"github.com/projectatomic/libpod/cmd/podman/varlink"
	"github.com/projectatomic/libpod/libpod"
)

// ListContainers ...
func (i *LibpodAPI) ListContainers(call ioprojectatomicpodman.VarlinkCall) error {
	var (
		listContainers []ioprojectatomicpodman.ListContainerData
	)

	runtime, err := libpodruntime.GetRuntime(i.Cli)
	if err != nil {
		return call.ReplyRuntimeError(err.Error())
	}
	containers, err := runtime.GetAllContainers()
	if err != nil {
		return call.ReplyErrorOccurred(err.Error())
	}
	opts := batchcontainer.PsOptions{
		Namespace: true,
		Size:      true,
	}
	for _, ctr := range containers {
		batchInfo, err := batchcontainer.BatchContainerOp(ctr, opts)
		if err != nil {
			return call.ReplyErrorOccurred(err.Error())
		}

		listContainers = append(listContainers, makeListContainer(ctr.ID(), batchInfo))
	}
	return call.ReplyListContainers(listContainers)
}

// GetContainer ...
func (i *LibpodAPI) GetContainer(call ioprojectatomicpodman.VarlinkCall, name string) error {
	runtime, err := libpodruntime.GetRuntime(i.Cli)
	if err != nil {
		return call.ReplyRuntimeError(err.Error())
	}
	ctr, err := runtime.LookupContainer(name)
	if err != nil {
		return call.ReplyContainerNotFound(name)
	}
	opts := batchcontainer.PsOptions{
		Namespace: true,
		Size:      true,
	}
	batchInfo, err := batchcontainer.BatchContainerOp(ctr, opts)
	if err != nil {
		return call.ReplyErrorOccurred(err.Error())
	}
	return call.ReplyGetContainer(makeListContainer(ctr.ID(), batchInfo))
}

// CreateContainer ...
func (i *LibpodAPI) CreateContainer(call ioprojectatomicpodman.VarlinkCall) error {
	return call.ReplyMethodNotImplemented("CreateContainer")
}

// InspectContainer ...
func (i *LibpodAPI) InspectContainer(call ioprojectatomicpodman.VarlinkCall, name string) error {
	runtime, err := libpodruntime.GetRuntime(i.Cli)
	if err != nil {
		return call.ReplyRuntimeError(err.Error())
	}
	ctr, err := runtime.LookupContainer(name)
	if err != nil {
		return call.ReplyContainerNotFound(name)
	}
	inspectInfo, err := ctr.Inspect(true)
	if err != nil {
		return call.ReplyErrorOccurred(err.Error())
	}
	b, err := json.Marshal(inspectInfo)
	if err != nil {
		return call.ReplyErrorOccurred(fmt.Sprintf("unable to serialize"))
	}
	return call.ReplyInspectContainer(string(b))
}

// ListContainerProcesses ...
func (i *LibpodAPI) ListContainerProcesses(call ioprojectatomicpodman.VarlinkCall, name string, opts []string) error {
	runtime, err := libpodruntime.GetRuntime(i.Cli)
	if err != nil {
		return call.ReplyRuntimeError(err.Error())
	}
	ctr, err := runtime.LookupContainer(name)
	if err != nil {
		return call.ReplyContainerNotFound(name)
	}
	containerState, err := ctr.State()
	if err != nil {
		return call.ReplyErrorOccurred(err.Error())
	}
	if containerState != libpod.ContainerStateRunning {
		return call.ReplyErrorOccurred(fmt.Sprintf("container %s is not running", name))
	}
	var psArgs []string
	psOpts := []string{"-o", "uid,pid,ppid,c,stime,tname,time,cmd"}
	if len(opts) > 1 {
		psOpts = opts
	}
	psArgs = append(psArgs, psOpts...)
	psOutput, err := ctr.GetContainerPidInformation(psArgs)
	if err != nil {
		return call.ReplyErrorOccurred(err.Error())
	}
	return call.ReplyListContainerProcesses(psOutput)
}

// GetContainerLogs ...
func (i *LibpodAPI) GetContainerLogs(call ioprojectatomicpodman.VarlinkCall, name string) error {
	var logs []string
	runtime, err := libpodruntime.GetRuntime(i.Cli)
	if err != nil {
		return call.ReplyRuntimeError(err.Error())
	}
	ctr, err := runtime.LookupContainer(name)
	if err != nil {
		return call.ReplyContainerNotFound(name)
	}
	logPath := ctr.LogPath()

	containerState, err := ctr.State()
	if err != nil {
		return call.ReplyErrorOccurred(err.Error())
	}
	if _, err := os.Stat(logPath); err != nil {
		if containerState == libpod.ContainerStateConfigured {
			return call.ReplyGetContainerLogs(logs)
		}
	}
	file, err := os.Open(logPath)
	if err != nil {
		return errors.Wrapf(err, "unable to read container log file")
	}
	defer file.Close()
	reader := bufio.NewReader(file)
	for {
		line, err := reader.ReadString('\n')
		if err != nil {
			break
		}
		logs = append(logs, line)
	}
	return call.ReplyGetContainerLogs(logs)
}

// ListContainerChanges ...
func (i *LibpodAPI) ListContainerChanges(call ioprojectatomicpodman.VarlinkCall, name string) error {
	runtime, err := libpodruntime.GetRuntime(i.Cli)
	if err != nil {
		return call.ReplyRuntimeError(err.Error())
	}
	changes, err := runtime.GetDiff("", name)
	if err != nil {
		return call.ReplyErrorOccurred(err.Error())
	}

	m := make(map[string]string)
	for _, change := range changes {
		m[change.Path] = change.Kind.String()
	}
	return call.ReplyListContainerChanges(m)
}

// ExportContainer ...
func (i *LibpodAPI) ExportContainer(call ioprojectatomicpodman.VarlinkCall, name, path string) error {
	runtime, err := libpodruntime.GetRuntime(i.Cli)
	if err != nil {
		return call.ReplyRuntimeError(err.Error())
	}
	ctr, err := runtime.LookupContainer(name)
	if err != nil {
		return call.ReplyContainerNotFound(name)
	}
	if err := ctr.Export(path); err != nil {
		return call.ReplyErrorOccurred(err.Error())
	}
	return call.ReplyExportContainer(path)
}

// GetContainerStats ...
func (i *LibpodAPI) GetContainerStats(call ioprojectatomicpodman.VarlinkCall, name string) error {
	runtime, err := libpodruntime.GetRuntime(i.Cli)
	if err != nil {
		return call.ReplyRuntimeError(err.Error())
	}
	ctr, err := runtime.LookupContainer(name)
	if err != nil {
		return call.ReplyContainerNotFound(name)
	}
	containerStats, err := ctr.GetContainerStats(&libpod.ContainerStats{})
	if err != nil {
		return call.ReplyErrorOccurred(err.Error())
	}
	cs := ioprojectatomicpodman.ContainerStats{
		Id:           ctr.ID(),
		Name:         ctr.Name(),
		Cpu:          containerStats.CPU,
		Cpu_nano:     int64(containerStats.CPUNano),
		System_nano:  int64(containerStats.SystemNano),
		Mem_usage:    int64(containerStats.MemUsage),
		Mem_limit:    int64(containerStats.MemLimit),
		Mem_perc:     containerStats.MemPerc,
		Net_input:    int64(containerStats.NetInput),
		Net_output:   int64(containerStats.NetOutput),
		Block_input:  int64(containerStats.BlockInput),
		Block_output: int64(containerStats.BlockOutput),
		Pids:         int64(containerStats.PIDs),
	}
	return call.ReplyGetContainerStats(cs)
}

// ResizeContainerTty ...
func (i *LibpodAPI) ResizeContainerTty(call ioprojectatomicpodman.VarlinkCall) error {
	return call.ReplyMethodNotImplemented("ResizeContainerTty")
}

// StartContainer ...
func (i *LibpodAPI) StartContainer(call ioprojectatomicpodman.VarlinkCall) error {
	return call.ReplyMethodNotImplemented("StartContainer")
}

// StopContainer ...
func (i *LibpodAPI) StopContainer(call ioprojectatomicpodman.VarlinkCall, name string, timeout int64) error {
	runtime, err := libpodruntime.GetRuntime(i.Cli)
	if err != nil {
		return call.ReplyRuntimeError(err.Error())
	}
	ctr, err := runtime.LookupContainer(name)
	if err != nil {
		return call.ReplyContainerNotFound(name)
	}
	if err := ctr.StopWithTimeout(uint(timeout)); err != nil && err != libpod.ErrCtrStopped {
		return call.ReplyErrorOccurred(err.Error())
	}
	return call.ReplyStopContainer(ctr.ID())
}

// RestartContainer ...
func (i *LibpodAPI) RestartContainer(call ioprojectatomicpodman.VarlinkCall, name string, timeout int64) error {
	runtime, err := libpodruntime.GetRuntime(i.Cli)
	if err != nil {
		return call.ReplyRuntimeError(err.Error())
	}
	ctr, err := runtime.LookupContainer(name)
	if err != nil {
		return call.ReplyContainerNotFound(name)
	}
	if err := ctr.RestartWithTimeout(getContext(), uint(timeout)); err != nil {
		return call.ReplyErrorOccurred(err.Error())
	}
	return call.ReplyRestartContainer(ctr.ID())
}

// KillContainer kills a running container.  If you want to use the default SIGTERM signal, just send a -1
// for the signal arg.
func (i *LibpodAPI) KillContainer(call ioprojectatomicpodman.VarlinkCall, name string, signal int64) error {
	var killSignal uint = uint(syscall.SIGTERM)
	if signal != -1 {
		killSignal = uint(signal)
	}
	runtime, err := libpodruntime.GetRuntime(i.Cli)
	if err != nil {
		return call.ReplyRuntimeError(err.Error())
	}
	ctr, err := runtime.LookupContainer(name)
	if err != nil {
		return call.ReplyContainerNotFound(name)
	}
	if err := ctr.Kill(killSignal); err != nil {
		return call.ReplyErrorOccurred(err.Error())
	}
	return call.ReplyKillContainer(ctr.ID())
}

// UpdateContainer ...
func (i *LibpodAPI) UpdateContainer(call ioprojectatomicpodman.VarlinkCall) error {
	return call.ReplyMethodNotImplemented("UpdateContainer")
}

// RenameContainer ...
func (i *LibpodAPI) RenameContainer(call ioprojectatomicpodman.VarlinkCall) error {
	return call.ReplyMethodNotImplemented("RenameContainer")
}

// PauseContainer ...
func (i *LibpodAPI) PauseContainer(call ioprojectatomicpodman.VarlinkCall, name string) error {
	runtime, err := libpodruntime.GetRuntime(i.Cli)
	if err != nil {
		return call.ReplyRuntimeError(err.Error())
	}
	ctr, err := runtime.LookupContainer(name)
	if err != nil {
		return call.ReplyContainerNotFound(name)
	}
	if err := ctr.Pause(); err != nil {
		return call.ReplyErrorOccurred(err.Error())
	}
	return call.ReplyPauseContainer(ctr.ID())
}

// UnpauseContainer ...
func (i *LibpodAPI) UnpauseContainer(call ioprojectatomicpodman.VarlinkCall, name string) error {
	runtime, err := libpodruntime.GetRuntime(i.Cli)
	if err != nil {
		return call.ReplyRuntimeError(err.Error())
	}
	ctr, err := runtime.LookupContainer(name)
	if err != nil {
		return call.ReplyContainerNotFound(name)
	}
	if err := ctr.Unpause(); err != nil {
		return call.ReplyErrorOccurred(err.Error())
	}
	return call.ReplyUnpauseContainer(ctr.ID())
}

// AttachToContainer ...
// TODO: DO we also want a different one for websocket?
func (i *LibpodAPI) AttachToContainer(call ioprojectatomicpodman.VarlinkCall) error {
	return call.ReplyMethodNotImplemented("AttachToContainer")
}

// WaitContainer ...
func (i *LibpodAPI) WaitContainer(call ioprojectatomicpodman.VarlinkCall, name string) error {
	runtime, err := libpodruntime.GetRuntime(i.Cli)
	if err != nil {
		return call.ReplyRuntimeError(err.Error())
	}
	ctr, err := runtime.LookupContainer(name)
	if err != nil {
		return call.ReplyContainerNotFound(name)
	}
	exitCode, err := ctr.Wait()
	if err != nil {
		return call.ReplyErrorOccurred(err.Error())
	}
	return call.ReplyWaitContainer(int64(exitCode))

}

// RemoveContainer ...
func (i *LibpodAPI) RemoveContainer(call ioprojectatomicpodman.VarlinkCall, name string, force bool) error {
	runtime, err := libpodruntime.GetRuntime(i.Cli)
	if err != nil {
		return call.ReplyRuntimeError(err.Error())
	}
	ctr, err := runtime.LookupContainer(name)
	if err != nil {
		return call.ReplyContainerNotFound(name)
	}
	if err := runtime.RemoveContainer(ctr, force); err != nil {
		return call.ReplyErrorOccurred(err.Error())
	}
	return call.ReplyRemoveContainer(ctr.ID())

}

// DeleteStoppedContainers ...
func (i *LibpodAPI) DeleteStoppedContainers(call ioprojectatomicpodman.VarlinkCall) error {
	var deletedContainers []string
	runtime, err := libpodruntime.GetRuntime(i.Cli)
	if err != nil {
		return call.ReplyRuntimeError(err.Error())
	}
	containers, err := runtime.GetAllContainers()
	if err != nil {
		return call.ReplyErrorOccurred(err.Error())
	}
	for _, ctr := range containers {
		state, err := ctr.State()
		if err != nil {
			return call.ReplyErrorOccurred(err.Error())
		}
		if state != libpod.ContainerStateRunning {
			if err := runtime.RemoveContainer(ctr, false); err != nil {
				return call.ReplyErrorOccurred(err.Error())
			}
			deletedContainers = append(deletedContainers, ctr.ID())
		}
	}
	return call.ReplyDeleteStoppedContainers(deletedContainers)
}
