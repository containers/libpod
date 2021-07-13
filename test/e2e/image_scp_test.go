package integration

import (
	"io/ioutil"
	"os"

	"github.com/containers/common/pkg/config"
	. "github.com/containers/podman/v3/test/utils"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

var _ = Describe("podman image scp", func() {
	ConfPath := struct {
		Value string
		IsSet bool
	}{}
	var (
		tempdir    string
		podmanTest *PodmanTestIntegration
	)

	BeforeEach(func() {
		ConfPath.Value, ConfPath.IsSet = os.LookupEnv("CONTAINERS_CONF")
		conf, err := ioutil.TempFile("", "containersconf")
		if err != nil {
			panic(err)
		}
		os.Setenv("CONTAINERS_CONF", conf.Name())
		tempdir, err = CreateTempDirInTempDir()
		if err != nil {
			os.Exit(1)
		}
		podmanTest = PodmanTestCreate(tempdir)
		podmanTest.Setup()
	})

	AfterEach(func() {
		podmanTest.Cleanup()
		os.Remove(os.Getenv("CONTAINERS_CONF"))
		if ConfPath.IsSet {
			os.Setenv("CONTAINERS_CONF", ConfPath.Value)
		} else {
			os.Unsetenv("CONTAINERS_CONF")
		}
		f := CurrentGinkgoTestDescription()
		processTestResult(f)

	})

	It("podman-remote image scp oci flag", func() {
		if !IsRemote() {
			Skip("Remote only test")
		}
		podmanTest.StopRemoteService()
		podmanTest.StartRemoteService()
		cmd := []string{"system", "connection", "add",
			"--default",
			"remote",
			"unix:///run/podman/podman.sock",
		}
		session := podmanTest.Podman(cmd)
		session.WaitWithDefaultTimeout()
		Expect(session.ExitCode()).Should(Equal(0))

		scp := podmanTest.Podman([]string{"image", "scp", "--format", "oci-archive", ALPINE})
		scp.WaitWithDefaultTimeout()
		Expect(scp.ExitCode()).To(Equal(0))
	})

	It("podman image scp quiet flag", func() {
		if IsRemote() {
			podmanTest.StopRemoteService()
			podmanTest.StartRemoteService()
		}
		cmd := []string{"system", "connection", "add",
			"--default",
			"remote",
			"unix:///run/podman/podman.sock",
		}
		session := podmanTest.Podman(cmd)
		session.WaitWithDefaultTimeout()
		Expect(session.ExitCode()).Should(Equal(0))

		scp := podmanTest.Podman([]string{"image", "scp", "-q", ALPINE})
		scp.WaitWithDefaultTimeout()
		Expect(scp.ExitCode()).To(Equal(0))
	})

	It("podman image scp bogus image", func() {

		scp := podmanTest.Podman([]string{"image", "scp", "FOOBAR"})
		scp.WaitWithDefaultTimeout()
		Expect(scp).To(ExitWithError())
	})

	It("podman-remote image scp to directory with oci format", func() {
		if !IsRemote() {
			Skip("Remote only test")
		}

		podmanTest.StopRemoteService()
		podmanTest.StartRemoteService()
		cmd := []string{"system", "connection", "add",
			"--default",
			"remote",
			"unix:///run/podman/podman.sock",
		}
		session := podmanTest.Podman(cmd)
		session.WaitWithDefaultTimeout()
		Expect(session.ExitCode()).Should(Equal(0))
		scp := podmanTest.Podman([]string{"image", "scp", "--format", "oci-archive", ALPINE})
		scp.WaitWithDefaultTimeout()
		Expect(scp.ExitCode()).To(Equal(0))
	})

	It("podman image scp with proper connection", func() {
		if IsRemote() {
			Skip("this test is only for non-remote")
		}
		cmd := []string{"system", "connection", "add",
			"--default",
			"--identity", "~/.ssh/id_rsa",
			"QA",
			"ssh://root@server.fubar.com:2222/run/podman/podman.sock",
		}
		session := podmanTest.Podman(cmd)
		session.WaitWithDefaultTimeout()
		Expect(session.ExitCode()).Should(Equal(0))

		cfg, err := config.ReadCustomConfig()
		Expect(err).ShouldNot(HaveOccurred())
		Expect(cfg.Engine.ActiveService).To(Equal("QA"))
		Expect(cfg.Engine.ServiceDestinations["QA"]).To(Equal(
			config.Destination{
				URI:      "ssh://root@server.fubar.com:2222/run/podman/podman.sock",
				Identity: "~/.ssh/id_rsa",
			},
		))

		scp := podmanTest.Podman([]string{"image", "scp", "--format", "oci-dir", ALPINE, "QA"})
		scp.WaitWithDefaultTimeout()
		Expect(scp).To(ExitWithError())

	})

})
