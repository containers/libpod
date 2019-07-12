package createconfig

import (
	"os"
	"path/filepath"
	"strings"

	"github.com/containers/libpod/libpod"
	"github.com/containers/libpod/pkg/cgroups"
	"github.com/containers/libpod/pkg/rootless"
	pmount "github.com/containers/storage/pkg/mount"
	"github.com/docker/docker/oci/caps"
	"github.com/docker/go-units"
	"github.com/opencontainers/runc/libcontainer/user"
	spec "github.com/opencontainers/runtime-spec/specs-go"
	"github.com/opencontainers/runtime-tools/generate"
	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"
)

const cpuPeriod = 100000

type systemUlimit struct {
	name string
	max  uint64
	cur  uint64
}

func getAvailableGids() (int64, error) {
	idMap, err := user.ParseIDMapFile("/proc/self/gid_map")
	if err != nil {
		return 0, err
	}
	count := int64(0)
	for _, r := range idMap {
		count += r.Count
	}
	return count, nil
}

// CreateConfigToOCISpec parses information needed to create a container into an OCI runtime spec
func (config *CreateConfig) createConfigToOCISpec(runtime *libpod.Runtime, userMounts []spec.Mount) (*spec.Spec, error) {
	cgroupPerm := "ro"
	g, err := generate.New("linux")
	if err != nil {
		return nil, err
	}
	// Remove the default /dev/shm mount to ensure we overwrite it
	g.RemoveMount("/dev/shm")
	g.HostSpecific = true
	addCgroup := true
	canMountSys := true

	isRootless := rootless.IsRootless()
	inUserNS := isRootless || (len(config.IDMappings.UIDMap) > 0 || len(config.IDMappings.GIDMap) > 0) && !config.UsernsMode.IsHost()

	if inUserNS && config.NetMode.IsHost() {
		canMountSys = false
	}

	if config.Privileged && canMountSys {
		cgroupPerm = "rw"
		g.RemoveMount("/sys")
		sysMnt := spec.Mount{
			Destination: "/sys",
			Type:        "sysfs",
			Source:      "sysfs",
			Options:     []string{"rprivate", "nosuid", "noexec", "nodev", "rw"},
		}
		g.AddMount(sysMnt)
	} else if !canMountSys {
		addCgroup = false
		g.RemoveMount("/sys")
		r := "ro"
		if config.Privileged {
			r = "rw"
		}
		sysMnt := spec.Mount{
			Destination: "/sys",
			Type:        TypeBind,
			Source:      "/sys",
			Options:     []string{"rprivate", "nosuid", "noexec", "nodev", r, "rbind"},
		}
		g.AddMount(sysMnt)
		if !config.Privileged && isRootless {
			g.AddLinuxMaskedPaths("/sys/kernel")
		}
	}
	gid5Available := true
	if isRootless {
		nGids, err := getAvailableGids()
		if err != nil {
			return nil, err
		}
		gid5Available = nGids >= 5
	}
	// When using a different user namespace, check that the GID 5 is mapped inside
	// the container.
	if gid5Available && len(config.IDMappings.GIDMap) > 0 {
		mappingFound := false
		for _, r := range config.IDMappings.GIDMap {
			if r.ContainerID <= 5 && 5 < r.ContainerID+r.Size {
				mappingFound = true
				break
			}
		}
		if !mappingFound {
			gid5Available = false
		}

	}
	if !gid5Available {
		// If we have no GID mappings, the gid=5 default option would fail, so drop it.
		g.RemoveMount("/dev/pts")
		devPts := spec.Mount{
			Destination: "/dev/pts",
			Type:        "devpts",
			Source:      "devpts",
			Options:     []string{"rprivate", "nosuid", "noexec", "newinstance", "ptmxmode=0666", "mode=0620"},
		}
		g.AddMount(devPts)
	}

	if inUserNS && config.IpcMode.IsHost() {
		g.RemoveMount("/dev/mqueue")
		devMqueue := spec.Mount{
			Destination: "/dev/mqueue",
			Type:        TypeBind,
			Source:      "/dev/mqueue",
			Options:     []string{"bind", "nosuid", "noexec", "nodev"},
		}
		g.AddMount(devMqueue)
	}
	if inUserNS && config.PidMode.IsHost() {
		g.RemoveMount("/proc")
		procMount := spec.Mount{
			Destination: "/proc",
			Type:        TypeBind,
			Source:      "/proc",
			Options:     []string{"rbind", "nosuid", "noexec", "nodev"},
		}
		g.AddMount(procMount)
	}

	if addCgroup {
		cgroupMnt := spec.Mount{
			Destination: "/sys/fs/cgroup",
			Type:        "cgroup",
			Source:      "cgroup",
			Options:     []string{"rprivate", "nosuid", "noexec", "nodev", "relatime", cgroupPerm},
		}
		g.AddMount(cgroupMnt)
	}
	g.SetProcessCwd(config.WorkDir)
	g.SetProcessArgs(config.Command)
	g.SetProcessTerminal(config.Tty)

	for key, val := range config.Annotations {
		g.AddAnnotation(key, val)
	}
	g.SetRootReadonly(config.ReadOnlyRootfs)

	if config.HTTPProxy {
		for _, envSpec := range []string{
			"http_proxy",
			"HTTP_PROXY",
			"https_proxy",
			"HTTPS_PROXY",
			"ftp_proxy",
			"FTP_PROXY",
			"no_proxy",
			"NO_PROXY",
		} {
			envVal := os.Getenv(envSpec)
			if envVal != "" {
				g.AddProcessEnv(envSpec, envVal)
			}
		}
	}

	hostname := config.Hostname
	if hostname == "" && (config.NetMode.IsHost() || config.UtsMode.IsHost()) {
		hostname, err = os.Hostname()
		if err != nil {
			return nil, errors.Wrap(err, "unable to retrieve hostname")
		}
	}
	g.RemoveHostname()
	if config.Hostname != "" || !config.UtsMode.IsHost() {
		// Set the hostname in the OCI configuration only
		// if specified by the user or if we are creating
		// a new UTS namespace.
		g.SetHostname(hostname)
	}
	g.AddProcessEnv("HOSTNAME", hostname)

	for sysctlKey, sysctlVal := range config.Sysctl {
		g.AddLinuxSysctl(sysctlKey, sysctlVal)
	}
	g.AddProcessEnv("container", "podman")

	addedResources := false

	// RESOURCES - MEMORY
	if config.Resources.Memory != 0 {
		g.SetLinuxResourcesMemoryLimit(config.Resources.Memory)
		// If a swap limit is not explicitly set, also set a swap limit
		// Default to double the memory limit
		if config.Resources.MemorySwap == 0 {
			g.SetLinuxResourcesMemorySwap(2 * config.Resources.Memory)
		}
		addedResources = true
	}
	if config.Resources.MemoryReservation != 0 {
		g.SetLinuxResourcesMemoryReservation(config.Resources.MemoryReservation)
		addedResources = true
	}
	if config.Resources.MemorySwap != 0 {
		g.SetLinuxResourcesMemorySwap(config.Resources.MemorySwap)
		addedResources = true
	}
	if config.Resources.KernelMemory != 0 {
		g.SetLinuxResourcesMemoryKernel(config.Resources.KernelMemory)
		addedResources = true
	}
	if config.Resources.MemorySwappiness != -1 {
		g.SetLinuxResourcesMemorySwappiness(uint64(config.Resources.MemorySwappiness))
		addedResources = true
	}
	g.SetLinuxResourcesMemoryDisableOOMKiller(config.Resources.DisableOomKiller)
	g.SetProcessOOMScoreAdj(config.Resources.OomScoreAdj)

	// RESOURCES - CPU
	if config.Resources.CPUShares != 0 {
		g.SetLinuxResourcesCPUShares(config.Resources.CPUShares)
		addedResources = true
	}
	if config.Resources.CPUQuota != 0 {
		g.SetLinuxResourcesCPUQuota(config.Resources.CPUQuota)
		addedResources = true
	}
	if config.Resources.CPUPeriod != 0 {
		g.SetLinuxResourcesCPUPeriod(config.Resources.CPUPeriod)
		addedResources = true
	}
	if config.Resources.CPUs != 0 {
		g.SetLinuxResourcesCPUPeriod(cpuPeriod)
		g.SetLinuxResourcesCPUQuota(int64(config.Resources.CPUs * cpuPeriod))
		addedResources = true
	}
	if config.Resources.CPURtRuntime != 0 {
		g.SetLinuxResourcesCPURealtimeRuntime(config.Resources.CPURtRuntime)
		addedResources = true
	}
	if config.Resources.CPURtPeriod != 0 {
		g.SetLinuxResourcesCPURealtimePeriod(config.Resources.CPURtPeriod)
		addedResources = true
	}
	if config.Resources.CPUsetCPUs != "" {
		g.SetLinuxResourcesCPUCpus(config.Resources.CPUsetCPUs)
		addedResources = true
	}
	if config.Resources.CPUsetMems != "" {
		g.SetLinuxResourcesCPUMems(config.Resources.CPUsetMems)
		addedResources = true
	}

	// Devices
	if config.Privileged {
		// If privileged, we need to add all the host devices to the
		// spec.  We do not add the user provided ones because we are
		// already adding them all.
		if !rootless.IsRootless() {
			if err := config.AddPrivilegedDevices(&g); err != nil {
				return nil, err
			}
		}
	} else {
		for _, devicePath := range config.Devices {
			if err := devicesFromPath(&g, devicePath); err != nil {
				return nil, err
			}
		}
	}

	for _, uidmap := range config.IDMappings.UIDMap {
		g.AddLinuxUIDMapping(uint32(uidmap.HostID), uint32(uidmap.ContainerID), uint32(uidmap.Size))
	}
	for _, gidmap := range config.IDMappings.GIDMap {
		g.AddLinuxGIDMapping(uint32(gidmap.HostID), uint32(gidmap.ContainerID), uint32(gidmap.Size))
	}
	// SECURITY OPTS
	g.SetProcessNoNewPrivileges(config.NoNewPrivs)

	if !config.Privileged {
		g.SetProcessApparmorProfile(config.ApparmorProfile)
	}

	blockAccessToKernelFilesystems(config, &g)

	// RESOURCES - PIDS
	if config.Resources.PidsLimit != 0 {
		g.SetLinuxResourcesPidsLimit(config.Resources.PidsLimit)
		addedResources = true
	}

	for name, val := range config.Env {
		g.AddProcessEnv(name, val)
	}

	if err := addRlimits(config, &g); err != nil {
		return nil, err
	}

	if err := addPidNS(config, &g); err != nil {
		return nil, err
	}

	if err := addUserNS(config, &g); err != nil {
		return nil, err
	}

	if err := addNetNS(config, &g); err != nil {
		return nil, err
	}

	if err := addUTSNS(config, &g); err != nil {
		return nil, err
	}

	if err := addIpcNS(config, &g); err != nil {
		return nil, err
	}
	configSpec := g.Config

	// HANDLE CAPABILITIES
	// NOTE: Must happen before SECCOMP
	if !config.Privileged {
		if err := setupCapabilities(config, configSpec); err != nil {
			return nil, err
		}
	} else {
		g.SetupPrivileged(true)
	}

	// HANDLE SECCOMP

	if config.SeccompProfilePath != "unconfined" {
		seccompConfig, err := getSeccompConfig(config, configSpec)
		if err != nil {
			return nil, err
		}
		configSpec.Linux.Seccomp = seccompConfig
	}

	// Clear default Seccomp profile from Generator for privileged containers
	if config.SeccompProfilePath == "unconfined" || config.Privileged {
		configSpec.Linux.Seccomp = nil
	}

	// BIND MOUNTS
	configSpec.Mounts = supercedeUserMounts(userMounts, configSpec.Mounts)
	// Process mounts to ensure correct options
	configSpec.Mounts = initFSMounts(configSpec.Mounts)

	// BLOCK IO
	blkio, err := config.CreateBlockIO()
	if err != nil {
		return nil, errors.Wrapf(err, "error creating block io")
	}
	if blkio != nil {
		configSpec.Linux.Resources.BlockIO = blkio
		addedResources = true
	}

	if rootless.IsRootless() {
		cgroup2, err := cgroups.IsCgroup2UnifiedMode()
		if err != nil {
			return nil, err
		}
		if addedResources && !cgroup2 {
			return nil, errors.New("invalid configuration, cannot set resources with rootless containers not using cgroups v2 unified mode")
		}
		if !cgroup2 {
			// Force the resources block to be empty instead of having default values.
			configSpec.Linux.Resources = &spec.LinuxResources{}
		}
	}

	// Make sure that the bind mounts keep options like nosuid, noexec, nodev.
	mounts, err := pmount.GetMounts()
	if err != nil {
		return nil, err
	}
	for i := range configSpec.Mounts {
		m := &configSpec.Mounts[i]
		isBind := false
		for _, o := range m.Options {
			if o == "bind" || o == "rbind" {
				isBind = true
				break
			}
		}
		if !isBind {
			continue
		}
		mount, err := findMount(m.Source, mounts)
		if err != nil {
			return nil, err
		}
		if mount == nil {
			continue
		}
	next_option:
		for _, o := range strings.Split(mount.Opts, ",") {
			if o == "nosuid" || o == "noexec" || o == "nodev" {
				for _, e := range m.Options {
					if e == o {
						continue next_option
					}
				}
				m.Options = append(m.Options, o)
			}
		}
	}

	return configSpec, nil
}

func findMount(target string, mounts []*pmount.Info) (*pmount.Info, error) {
	var err error
	target, err = filepath.Abs(target)
	if err != nil {
		return nil, errors.Wrapf(err, "cannot resolve %s", target)
	}
	var bestSoFar *pmount.Info
	for _, i := range mounts {
		if bestSoFar != nil && len(bestSoFar.Mountpoint) > len(i.Mountpoint) {
			// Won't be better than what we have already found
			continue
		}
		if strings.HasPrefix(target, i.Mountpoint) {
			bestSoFar = i
		}
	}
	return bestSoFar, nil
}

func blockAccessToKernelFilesystems(config *CreateConfig, g *generate.Generator) {
	if !config.Privileged {
		for _, mp := range []string{
			"/proc/acpi",
			"/proc/kcore",
			"/proc/keys",
			"/proc/latency_stats",
			"/proc/timer_list",
			"/proc/timer_stats",
			"/proc/sched_debug",
			"/proc/scsi",
			"/sys/firmware",
			"/sys/fs/selinux",
		} {
			g.AddLinuxMaskedPaths(mp)
		}

		if config.PidMode.IsHost() && rootless.IsRootless() {
			return
		}

		for _, rp := range []string{
			"/proc/asound",
			"/proc/bus",
			"/proc/fs",
			"/proc/irq",
			"/proc/sys",
			"/proc/sysrq-trigger",
		} {
			g.AddLinuxReadonlyPaths(rp)
		}
	}
}

func addPidNS(config *CreateConfig, g *generate.Generator) error {
	pidMode := config.PidMode
	if IsNS(string(pidMode)) {
		return g.AddOrReplaceLinuxNamespace(string(spec.PIDNamespace), NS(string(pidMode)))
	}
	if pidMode.IsHost() {
		return g.RemoveLinuxNamespace(string(spec.PIDNamespace))
	}
	if pidMode.IsContainer() {
		logrus.Debug("using container pidmode")
	}
	if IsPod(string(pidMode)) {
		logrus.Debug("using pod pidmode")
	}
	return nil
}

func addUserNS(config *CreateConfig, g *generate.Generator) error {
	if IsNS(string(config.UsernsMode)) {
		if err := g.AddOrReplaceLinuxNamespace(spec.UserNamespace, NS(string(config.UsernsMode))); err != nil {
			return err
		}

		// runc complains if no mapping is specified, even if we join another ns.  So provide a dummy mapping
		g.AddLinuxUIDMapping(uint32(0), uint32(0), uint32(1))
		g.AddLinuxGIDMapping(uint32(0), uint32(0), uint32(1))
	}

	if (len(config.IDMappings.UIDMap) > 0 || len(config.IDMappings.GIDMap) > 0) && !config.UsernsMode.IsHost() {
		if err := g.AddOrReplaceLinuxNamespace(spec.UserNamespace, ""); err != nil {
			return err
		}
	}
	return nil
}

func addNetNS(config *CreateConfig, g *generate.Generator) error {
	netMode := config.NetMode
	if netMode.IsHost() {
		logrus.Debug("Using host netmode")
		return g.RemoveLinuxNamespace(spec.NetworkNamespace)
	} else if netMode.IsNone() {
		logrus.Debug("Using none netmode")
		return nil
	} else if netMode.IsBridge() {
		logrus.Debug("Using bridge netmode")
		return nil
	} else if netMode.IsContainer() {
		logrus.Debug("Using container netmode")
		return nil
	} else if IsNS(string(netMode)) {
		logrus.Debug("Using ns netmode")
		return g.AddOrReplaceLinuxNamespace(spec.NetworkNamespace, NS(string(netMode)))
	} else if IsPod(string(netMode)) {
		logrus.Debug("Using pod netmode, unless pod is not sharing")
		return nil
	} else if netMode.IsSlirp4netns() {
		logrus.Debug("Using slirp4netns netmode")
		return nil
	} else if netMode.IsUserDefined() {
		logrus.Debug("Using user defined netmode")
		return nil
	}
	return errors.Errorf("unknown network mode")
}

func addUTSNS(config *CreateConfig, g *generate.Generator) error {
	utsMode := config.UtsMode
	if IsNS(string(utsMode)) {
		return g.AddOrReplaceLinuxNamespace(string(spec.UTSNamespace), NS(string(utsMode)))
	}
	if utsMode.IsHost() {
		return g.RemoveLinuxNamespace(spec.UTSNamespace)
	}
	return nil
}

func addIpcNS(config *CreateConfig, g *generate.Generator) error {
	ipcMode := config.IpcMode
	if IsNS(string(ipcMode)) {
		return g.AddOrReplaceLinuxNamespace(string(spec.IPCNamespace), NS(string(ipcMode)))
	}
	if ipcMode.IsHost() {
		return g.RemoveLinuxNamespace(spec.IPCNamespace)
	}
	if ipcMode.IsContainer() {
		logrus.Debug("Using container ipcmode")
	}

	return nil
}

func addRlimits(config *CreateConfig, g *generate.Generator) error {
	var (
		kernelMax  uint64 = 1048576
		isRootless        = rootless.IsRootless()
		nofileSet         = false
		nprocSet          = false
	)

	for _, u := range config.Resources.Ulimit {
		if u == "host" {
			if len(config.Resources.Ulimit) != 1 {
				return errors.New("ulimit can use host only once")
			}
			hostLimits, err := getHostRlimits()
			if err != nil {
				return err
			}
			for _, i := range hostLimits {
				g.AddProcessRlimits(i.name, i.max, i.cur)
			}
			break
		}

		ul, err := units.ParseUlimit(u)
		if err != nil {
			return errors.Wrapf(err, "ulimit option %q requires name=SOFT:HARD, failed to be parsed", u)
		}

		if ul.Name == "nofile" {
			nofileSet = true
		} else if ul.Name == "nproc" {
			nprocSet = true
		}

		g.AddProcessRlimits("RLIMIT_"+strings.ToUpper(ul.Name), uint64(ul.Hard), uint64(ul.Soft))
	}

	// If not explicitly overridden by the user, default number of open
	// files and number of processes to the maximum they can be set to
	// (without overriding a sysctl)
	if !nofileSet && !isRootless {
		g.AddProcessRlimits("RLIMIT_NOFILE", kernelMax, kernelMax)
	}
	if !nprocSet && !isRootless {
		g.AddProcessRlimits("RLIMIT_NPROC", kernelMax, kernelMax)
	}

	return nil
}

func setupCapabilities(config *CreateConfig, configSpec *spec.Spec) error {
	useNotRoot := func(user string) bool {
		if user == "" || user == "root" || user == "0" {
			return false
		}
		return true
	}

	var err error
	var caplist []string
	bounding := configSpec.Process.Capabilities.Bounding
	if useNotRoot(config.User) {
		configSpec.Process.Capabilities.Bounding = caplist
	}
	caplist, err = caps.TweakCapabilities(configSpec.Process.Capabilities.Bounding, config.CapAdd, config.CapDrop, nil, false)
	if err != nil {
		return err
	}

	configSpec.Process.Capabilities.Bounding = caplist
	configSpec.Process.Capabilities.Permitted = caplist
	configSpec.Process.Capabilities.Inheritable = caplist
	configSpec.Process.Capabilities.Effective = caplist
	configSpec.Process.Capabilities.Ambient = caplist
	if useNotRoot(config.User) {
		caplist, err = caps.TweakCapabilities(bounding, config.CapAdd, config.CapDrop, nil, false)
		if err != nil {
			return err
		}
	}
	configSpec.Process.Capabilities.Bounding = caplist
	return nil
}
