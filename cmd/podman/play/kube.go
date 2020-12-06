package pods

import (
	"fmt"
	"net"
	"os"

	"github.com/containers/common/pkg/auth"
	"github.com/containers/common/pkg/completion"
	"github.com/containers/image/v5/types"
	"github.com/containers/podman/v2/cmd/podman/common"
	"github.com/containers/podman/v2/cmd/podman/registry"
	"github.com/containers/podman/v2/cmd/podman/utils"
	"github.com/containers/podman/v2/pkg/domain/entities"
	"github.com/containers/podman/v2/pkg/util"
	"github.com/spf13/cobra"
)

// playKubeOptionsWrapper allows for separating CLI-only fields from API-only
// fields.
type playKubeOptionsWrapper struct {
	entities.PlayKubeOptions

	TLSVerifyCLI   bool
	CredentialsCLI string
	StartCLI       bool
	StaticIPCLI    string
}

var (
	// https://kubernetes.io/docs/reference/command-line-tools-reference/kubelet/
	defaultSeccompRoot = "/var/lib/kubelet/seccomp"
	kubeOptions        = playKubeOptionsWrapper{}
	kubeDescription    = `Command reads in a structured file of Kubernetes YAML.

  It creates the pod and containers described in the YAML.  The containers within the pod are then started and the ID of the new Pod is output.`

	kubeCmd = &cobra.Command{
		Use:               "kube [options] KUBEFILE",
		Short:             "Play a pod based on Kubernetes YAML.",
		Long:              kubeDescription,
		RunE:              kube,
		Args:              cobra.ExactArgs(1),
		ValidArgsFunction: completion.AutocompleteDefault,
		Example: `podman play kube nginx.yml
  podman play kube --creds user:password --seccomp-profile-root /custom/path apache.yml`,
	}
)

func init() {
	registry.Commands = append(registry.Commands, registry.CliCommand{
		Mode:    []entities.EngineMode{entities.ABIMode, entities.TunnelMode},
		Command: kubeCmd,
		Parent:  playCmd,
	})

	flags := kubeCmd.Flags()
	flags.SetNormalizeFunc(utils.AliasFlags)

	credsFlagName := "creds"
	flags.StringVar(&kubeOptions.CredentialsCLI, credsFlagName, "", "`Credentials` (USERNAME:PASSWORD) to use for authenticating to a registry")
	_ = kubeCmd.RegisterFlagCompletionFunc(credsFlagName, completion.AutocompleteNone)

	networkFlagName := "network"
	flags.StringVar(&kubeOptions.Network, networkFlagName, "", "Connect pod to CNI network(s)")
	_ = kubeCmd.RegisterFlagCompletionFunc(networkFlagName, common.AutocompleteNetworks)

	logDriverFlagName := "log-driver"
	flags.StringVar(&kubeOptions.LogDriver, logDriverFlagName, "", "Logging driver for the container")
	_ = kubeCmd.RegisterFlagCompletionFunc(logDriverFlagName, common.AutocompleteLogDriver)

	flags.BoolVarP(&kubeOptions.Quiet, "quiet", "q", false, "Suppress output information when pulling images")
	flags.BoolVar(&kubeOptions.TLSVerifyCLI, "tls-verify", true, "Require HTTPS and verify certificates when contacting registries")
	flags.BoolVar(&kubeOptions.StartCLI, "start", true, "Start the pod after creating it")

	authfileFlagName := "authfile"
	flags.StringVar(&kubeOptions.Authfile, authfileFlagName, auth.GetDefaultAuthFile(), "Path of the authentication file. Use REGISTRY_AUTH_FILE environment variable to override")
	_ = kubeCmd.RegisterFlagCompletionFunc(authfileFlagName, completion.AutocompleteDefault)

	staticIPFlagName := "ip"
	flags.StringVar(&kubeOptions.StaticIPCLI, staticIPFlagName, "", "Static IP address to assign to this pod")
	_ = kubeCmd.RegisterFlagCompletionFunc(staticIPFlagName, completion.AutocompleteDefault)

	if !registry.IsRemote() {

		certDirFlagName := "cert-dir"
		flags.StringVar(&kubeOptions.CertDir, certDirFlagName, "", "`Pathname` of a directory containing TLS certificates and keys")
		_ = kubeCmd.RegisterFlagCompletionFunc(certDirFlagName, completion.AutocompleteDefault)

		flags.StringVar(&kubeOptions.SignaturePolicy, "signature-policy", "", "`Pathname` of signature policy file (not usually used)")

		seccompProfileRootFlagName := "seccomp-profile-root"
		flags.StringVar(&kubeOptions.SeccompProfileRoot, seccompProfileRootFlagName, defaultSeccompRoot, "Directory path for seccomp profiles")
		_ = kubeCmd.RegisterFlagCompletionFunc(seccompProfileRootFlagName, completion.AutocompleteDefault)

		configmapFlagName := "configmap"
		flags.StringSliceVar(&kubeOptions.ConfigMaps, configmapFlagName, []string{}, "`Pathname` of a YAML file containing a kubernetes configmap")
		_ = kubeCmd.RegisterFlagCompletionFunc(configmapFlagName, completion.AutocompleteDefault)
	}
	_ = flags.MarkHidden("signature-policy")
}

func kube(cmd *cobra.Command, args []string) error {
	// TLS verification in c/image is controlled via a `types.OptionalBool`
	// which allows for distinguishing among set-true, set-false, unspecified
	// which is important to implement a sane way of dealing with defaults of
	// boolean CLI flags.
	if cmd.Flags().Changed("tls-verify") {
		kubeOptions.SkipTLSVerify = types.NewOptionalBool(!kubeOptions.TLSVerifyCLI)
	}
	if cmd.Flags().Changed("start") {
		kubeOptions.Start = types.NewOptionalBool(kubeOptions.StartCLI)
	}
	if kubeOptions.Authfile != "" {
		if _, err := os.Stat(kubeOptions.Authfile); err != nil {
			return err
		}
	}
	if kubeOptions.CredentialsCLI != "" {
		creds, err := util.ParseRegistryCreds(kubeOptions.CredentialsCLI)
		if err != nil {
			return err
		}
		kubeOptions.Username = creds.Username
		kubeOptions.Password = creds.Password
	}

	if kubeOptions.StaticIPCLI != "" {
		ipAddress := net.ParseIP(kubeOptions.StaticIPCLI)
		if ipAddress == nil {
			return fmt.Errorf("failed parsing ip address: %s", kubeOptions.StaticIPCLI)
		}
		kubeOptions.StaticIP = ipAddress
	}

	report, err := registry.ContainerEngine().PlayKube(registry.GetContext(), args[0], kubeOptions.PlayKubeOptions)
	if err != nil {
		return err
	}

	for _, pod := range report.Pods {
		for _, l := range pod.Logs {
			fmt.Fprintf(os.Stderr, l)
		}
	}

	for _, pod := range report.Pods {
		fmt.Printf("Pod:\n")
		fmt.Println(pod.ID)

		switch len(pod.Containers) {
		case 0:
			continue
		case 1:
			fmt.Printf("Container:\n")
		default:
			fmt.Printf("Containers:\n")
		}
		for _, ctr := range pod.Containers {
			fmt.Println(ctr)
		}
		// Empty line for space for next block
		fmt.Println()
	}

	return nil
}
