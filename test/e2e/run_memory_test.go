package integration

import (
	"os"

	. "github.com/containers/podman/v2/test/utils"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

var _ = Describe("Podman run memory", func() {
	var (
		tempdir    string
		err        error
		podmanTest *PodmanTestIntegration
	)

	BeforeEach(func() {
		SkipIfRootlessCgroupsV1("Setting Memory not supported on cgroupv1 for rootless users")

		SkipIfRootless("FIXME: This should work on cgroups V2 systems")
		tempdir, err = CreateTempDirInTempDir()
		if err != nil {
			os.Exit(1)
		}
		podmanTest = PodmanTestCreate(tempdir)
		podmanTest.Setup()
		podmanTest.SeedImages()
	})

	AfterEach(func() {
		podmanTest.Cleanup()
		f := CurrentGinkgoTestDescription()
		processTestResult(f)

	})

	It("podman run memory test", func() {
		var session *PodmanSessionIntegration

		if CGROUPSV2 {
			session = podmanTest.Podman([]string{"run", "--memory=40m", ALPINE, "sh", "-c", "cat /sys/fs/cgroup/$(sed -e 's|0::||' < /proc/self/cgroup)/memory.max"})
		} else {
			session = podmanTest.Podman([]string{"run", "--memory=40m", ALPINE, "cat", "/sys/fs/cgroup/memory/memory.limit_in_bytes"})
		}
		session.WaitWithDefaultTimeout()
		Expect(session.ExitCode()).To(Equal(0))
		Expect(session.OutputToString()).To(Equal("41943040"))
	})

	It("podman run memory-reservation test", func() {
		if podmanTest.Host.Distribution == "ubuntu" {
			Skip("Unable to perform test on Ubuntu distributions due to memory management")
		}

		var session *PodmanSessionIntegration

		if CGROUPSV2 {
			session = podmanTest.Podman([]string{"run", "--memory-reservation=40m", ALPINE, "sh", "-c", "cat /sys/fs/cgroup/$(sed -e 's|0::||' < /proc/self/cgroup)/memory.low"})
		} else {
			session = podmanTest.Podman([]string{"run", "--memory-reservation=40m", ALPINE, "cat", "/sys/fs/cgroup/memory/memory.soft_limit_in_bytes"})
		}

		session.WaitWithDefaultTimeout()
		Expect(session.ExitCode()).To(Equal(0))
		Expect(session.OutputToString()).To(Equal("41943040"))
	})

	It("podman run memory-swappiness test", func() {
		SkipIfCgroupV2("memory-swappiness not supported on cgroupV2")
		session := podmanTest.Podman([]string{"run", "--memory-swappiness=15", ALPINE, "cat", "/sys/fs/cgroup/memory/memory.swappiness"})
		session.WaitWithDefaultTimeout()
		Expect(session.ExitCode()).To(Equal(0))
		Expect(session.OutputToString()).To(Equal("15"))
	})

	It("podman run kernel-memory test", func() {
		if podmanTest.Host.Distribution == "ubuntu" {
			Skip("Unable to perform test on Ubuntu distributions due to memory management")
		}

		var session *PodmanSessionIntegration

		if CGROUPSV2 {
			session = podmanTest.Podman([]string{"run", "--memory-reservation=40m", ALPINE, "sh", "-c", "cat /sys/fs/cgroup/$(sed -e 's|0::||' < /proc/self/cgroup)/memory.low"})
		} else {
			session = podmanTest.Podman([]string{"run", "--memory-reservation=40m", ALPINE, "cat", "/sys/fs/cgroup/memory/memory.soft_limit_in_bytes"})
		}

		session.WaitWithDefaultTimeout()
		Expect(session.ExitCode()).To(Equal(0))
		Expect(session.OutputToString()).To(Equal("41943040"))
	})
})
