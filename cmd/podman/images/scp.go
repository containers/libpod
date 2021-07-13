package images

import (
	"bytes"
	"context"
	"fmt"
	"io/ioutil"
	"net"
	urlP "net/url"
	"os"
	"os/user"
	"strings"

	"github.com/containers/common/pkg/config"
	"github.com/containers/podman/v3/cmd/podman/common"
	"github.com/containers/podman/v3/cmd/podman/parse"
	"github.com/containers/podman/v3/cmd/podman/registry"
	"github.com/containers/podman/v3/libpod/define"
	"github.com/containers/podman/v3/pkg/domain/entities"
	"github.com/containers/podman/v3/pkg/domain/infra"
	t "github.com/containers/podman/v3/pkg/terminal"
	"github.com/containers/podman/v3/pkg/util"
	scpD "github.com/dtylman/scp"
	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"
	"github.com/spf13/cobra"
	"golang.org/x/crypto/ssh"
	"golang.org/x/crypto/ssh/agent"
)

var (
	validScpFormats = []string{define.OCIManifestDir, define.OCIArchive, define.V2s2ManifestDir, define.V2s2Archive}
)

var (
	saveScpDescription = `Securely copy an image from one host to another.`

	imageScpCommand = &cobra.Command{
		Use:   "scp [options] IMAGE [IMAGE...]",
		Long:  saveScpDescription,
		Short: "securely copy images",
		RunE:  scp,
		Args: func(cmd *cobra.Command, args []string) error {
			if len(args) < 1 {
				return errors.Errorf("need at least 1 argument")
			}
			format, err := cmd.Flags().GetString("format")
			if err != nil {
				return err
			}
			if !util.StringInSlice(format, validScpFormats) {
				return errors.Errorf("format value must be one of %s", strings.Join(validScpFormats, " "))
			}
			return nil
		},
		ValidArgsFunction: common.AutocompleteImages,
		Example:           `podman image scp myimage:latest otherhost::`,
	}
)

var (
	scpOpts entities.ImageScpOptions
)

func init() {
	registry.Commands = append(registry.Commands, registry.CliCommand{
		Command: imageScpCommand,
		Parent:  imageCmd,
	})
	scpFlags(imageScpCommand)
}

func scpFlags(cmd *cobra.Command) {
	flags := cmd.Flags()
	formatFlagName := "format"
	flags.StringVar(&scpOpts.Save.Format, formatFlagName, define.V2s2Archive, "Save image to oci-archive, docker-archive, docker-dir")
	_ = cmd.RegisterFlagCompletionFunc(formatFlagName, common.AutocompleteImageSaveFormat)
	flags.BoolVarP(&scpOpts.Save.Quiet, "quiet", "q", false, "Suppress the output")
}

func scp(cmd *cobra.Command, args []string) (finalErr error) {
	var (
		tags []string
		err  error
	)
	if scpOpts.Save.Quiet { // set quiet for both load and save
		scpOpts.Load.Quiet = true
	}
	scpOpts.ToFrom = true                 // default to loading to the ssh client
	f, err := ioutil.TempFile("", "temp") // open temp dir for load/save output
	if err != nil {
		return err
	}
	scpOpts.Save.Output = f.Name()
	scpOpts.Load.Input = scpOpts.Save.Output
	if err := parse.ValidateFileName(saveOpts.Output); err != nil {
		return err
	}
	confR, err := config.NewConfig("") // create a hand made config for the remote engine since we might use remote and native at once
	if err != nil {
		return errors.Wrapf(err, "could not make config")
	}
	abiEng, err := registry.NewImageEngine(cmd, args) // abi native engine
	if err != nil {
		return errors.Wrapf(err, "could not make abi engine")
	}
	url := ""
	iden := ""
	serv := map[string]config.Destination{}
	cfg, err := config.ReadCustomConfig() // get ready to set ssh destination if necessary
	if err != nil {
		return err
	}
	conn := ""
	if strings.Contains(args[0], "::") || len(args) == 2 { // if we have a connection name specified
		if len(args) == 2 && !(strings.Contains(args[1], "::") && strings.Contains(args[0], "::")) { // if an image is specified, this mean we are loading from our client
			conn = args[1]
		} else { // else we are loading from the ssh/remote client
			scpOpts.ToFrom = false
			conn = args[0]
		}
		splitEnv := strings.SplitN(conn, "::", 2)
		if len(splitEnv) == 2 {
			conn = splitEnv[0]
			if len(args) == 2 && !(strings.Contains(args[1], "::") && strings.Contains(args[0], "::")) && splitEnv[1] != "" { // if we have two arguments and its not remote -> remote
				scpOpts.Load.Input = splitEnv[1]
			} else {
				scpOpts.ImageName = splitEnv[1]
			}
		}
		if len(cfg.Engine.ServiceDestinations) == 0 { // no connections in list >:/
			logrus.Warnf("Unknown connection name given. Please use system connection add to specify the default remote socket location")
		}

		for name, data := range cfg.Engine.ServiceDestinations { // loop through all of the connections until we find outs
			if name == conn {
				iden = data.Identity
				url = data.URI
			}
		}
		if iden == "" && url == "" { // no match, warn user and do a manual connection.
			url = "ssh://" + conn
			logrus.Warnf("Unknown connection name given. Please use system connection add to specify the default remote socket location")
		}
	} else { // else we default to remote or whatever the default connection is
		connStr := cfg.Engine.ActiveService
		if len(cfg.Engine.ServiceDestinations) == 0 {
			logrus.Warnf("Unknown connection name given. Please use system connection add to specify the default remote socket location")
		}

		for name, data := range cfg.Engine.ServiceDestinations {
			if name == connStr {
				iden = data.Identity
				url = data.URI
			}
		}
		dst := config.Destination{}
		if iden != "" {
			dst.URI = url
			dst.Identity = iden
		} else {
			dst.URI = url
		}
		serv[connStr] = dst
	}
	optionsTunnel := entities.PodmanConfig{}
	confR.Engine = config.EngineConfig{Remote: true, CgroupManager: "cgroupfs", ServiceDestinations: serv} // pass the service dest (either remote or something else) to engine
	if iden != "" {
		optionsTunnel = entities.PodmanConfig{Config: confR, EngineMode: entities.TunnelMode, Remote: true, Identity: iden, URI: url}
	} else {
		optionsTunnel = entities.PodmanConfig{Config: confR, EngineMode: entities.TunnelMode, Remote: true, URI: url}
	}
	if scpOpts.ToFrom {
		abiErr := abiEng.Save(context.Background(), args[0], tags, scpOpts.Save) // save the image locally before loading it on remote, local, or ssh
		if abiErr != nil {
			errors.Wrapf(abiErr, "could not save image as specified")
		}
	}
	uri, err := urlP.Parse(url) // create an actual url to pass to exec command
	if err != nil {
		return err
	}
	if uri.User.Username() == "" {
		if uri.User, err = getUserInfo(uri); err != nil {
			return err
		}
	}
	rem := cmd.Root().Flags().Lookup("remote").Value // if remote was specified globally...
	remS := rem.String()
	if remS == "true" {
		tunEng, err := infra.NewImageEngine(&optionsTunnel) // means we are doing a tunnel load
		if err != nil {
			return (errors.Wrapf(err, "could not make tunnel engine"))
		}
		tunOut, err := tunEng.Load(context.Background(), scpOpts.Load)
		if err != nil {
			errors.Wrapf(err, "could not load saved image")
		}
		if tunOut != nil {
			out := tunOut.Names
			if len(tunOut.Names) == 0 {
				return errors.Wrapf(errors.New("Invalid Image given"), "could not get image")
			}
			fmt.Println("Loaded image(s): " + strings.Join(out, ","))
		}
	} else if len(args) == 2 && scpOpts.ToFrom { // else if a remote ssh connection was specified, means we need to go to exec command
		rep, err := execCommand(iden, uri, scpOpts.Save.Output, scpOpts.Load.Input, args[0], scpOpts.ToFrom)
		if err != nil {
			return err
		}
		fmt.Println(rep)
	} else if !scpOpts.ToFrom { // else if we want to load FROM the remote
		_, err := execCommand(iden, uri, scpOpts.Save.Output, scpOpts.ImageName, args[0], scpOpts.ToFrom)
		if err != nil {
			return err
		}
		if len(args) == 2 && !(strings.Contains(args[1], "::") && strings.Contains(args[0], "::")) { // if we don't want to do a remote -> remote then simply load
			report, err := abiEng.Load(context.Background(), scpOpts.Load)
			if err != nil {
				return err
			}
			fmt.Println("Loaded images(s): " + strings.Join(report.Names, ","))
		} else { // else we want to load remote -> remote
			rep, err := execCommand(iden, uri, scpOpts.Save.Output, scpOpts.Load.Input, scpOpts.ImageName, true) // execute the command from line 207 with ToFrom defaulting to true
			if err != nil {
				return err
			}
			fmt.Println(rep)
		}
	} else { // else native load
		rep, err := abiEng.Load(context.Background(), scpOpts.Load)
		if err != nil {
			return err
		}
		fmt.Println("Loaded images(s): " + strings.Join(rep.Names, ","))
	}
	defer os.Remove(f.Name())
	return nil
}

// execCommand takes ssh information and an image file path on the client, connects via ssh, executes
// an scp command and then calls podman image load on the remote machine given the new scp filepath
func execCommand(iden string, uri *urlP.URL, imageFileIn string, imageFileOut string, image string, ToFrom bool) (string, error) {
	var signers []ssh.Signer
	passwd, passwdSet := uri.User.Password()
	if iden != "" {
		value := iden
		s, err := t.PublicKey(value, []byte(passwd))
		if err != nil {
			return "", errors.Wrapf(err, "failed to read identity %q", value)
		}
		signers = append(signers, s)
		logrus.Debugf("SSH Ident Key %q %s %s", value, ssh.FingerprintSHA256(s.PublicKey()), s.PublicKey().Type())
	}
	if sock, found := os.LookupEnv("SSH_AUTH_SOCK"); found { // validate ssh information
		logrus.Debugf("Found SSH_AUTH_SOCK %q, ssh-agent signer enabled", sock)

		c, err := net.Dial("unix", sock)
		if err != nil {
			return "", err
		}
		agentSigners, err := agent.NewClient(c).Signers()
		if err != nil {
			return "", err
		}

		signers = append(signers, agentSigners...)

		if logrus.IsLevelEnabled(logrus.DebugLevel) {
			for _, s := range agentSigners {
				logrus.Debugf("SSH Agent Key %s %s", ssh.FingerprintSHA256(s.PublicKey()), s.PublicKey().Type())
			}
		}
	}
	var authMethods []ssh.AuthMethod
	if len(signers) > 0 {
		var dedup = make(map[string]ssh.Signer)
		for _, s := range signers {
			fp := ssh.FingerprintSHA256(s.PublicKey())
			if _, found := dedup[fp]; found {
				logrus.Debugf("Dedup SSH Key %s %s", ssh.FingerprintSHA256(s.PublicKey()), s.PublicKey().Type()) // public key info and validation
			}
			dedup[fp] = s
		}
		var uniq []ssh.Signer
		for _, s := range dedup {
			uniq = append(uniq, s)
		}
		authMethods = append(authMethods, ssh.PublicKeysCallback(func() ([]ssh.Signer, error) {
			return uniq, nil
		}))
	}
	if passwdSet { // proper password validation for ssh connection if necessary
		authMethods = append(authMethods, ssh.Password(passwd))
	}
	if len(authMethods) == 0 {
		authMethods = append(authMethods, ssh.PasswordCallback(func() (string, error) {
			pass, err := t.ReadPassword(fmt.Sprintf("%s's login password:", uri.User.Username()))
			return string(pass), err
		}))
	}
	cfg := &ssh.ClientConfig{ // configure ssh client
		User:            uri.User.Username(),
		Auth:            authMethods,
		HostKeyCallback: ssh.InsecureIgnoreHostKey(),
	}
	dial, err := ssh.Dial("tcp", uri.Host, cfg) // dial the client
	if err != nil {
		return "", errors.Wrapf(err, "failed to connect")
	}
	defer dial.Close()

	session, err := dial.NewSession() // connect to the dialed client
	if err != nil {
		return "", errors.Wrapf(err, "failed to create new ssh session on %q", uri.Host)
	}
	defer session.Close()
	if err != nil {
		return "", err
	}
	if ToFrom { // if we are loading to the remote/ssh client
		f, _ := os.Open(imageFileIn) // open image file to copy
		defer f.Close()
		n, err := scpD.CopyTo(dial, imageFileIn, imageFileOut)
		if err != nil {
			fmt.Println(n, "Bytes copied before error. Error while copying file ", err)
		}

		podman := "podman"
		if v, found := os.LookupEnv("PODMAN_BINARY"); found {
			podman = v // get path to podman bin
		}
		run := podman + " image load --input=" + imageFileOut // run ssh image load of the file copied via scp
		var buffer bytes.Buffer
		session.Stdout = &buffer
		if err := session.Run(run); err != nil { // run the command declared above
			return "", err
		}
		return buffer.String(), nil // return the output of image load on the machine, should be "Loaded Image(s)..."
	} // else we are loading from the ssh client
	podman := "podman"
	if v, found := os.LookupEnv("PODMAN_BINARY"); found {
		podman = v // get path to podman bin
	}
	run := podman + " image save " + imageFileOut + " --format=oci-archive --output=" + imageFileIn // run ssh image load of the file copied via scp. Files are reverse in thie case...
	var buffer bytes.Buffer
	session.Stdout = &buffer

	if err := session.Run(run); err != nil { // run the command declared above
		return "", err
	}
	n, err := scpD.CopyFrom(dial, imageFileIn, imageFileIn)
	if err != nil {
		fmt.Print(n, " bytes copied before err")
		return "", err
	}
	return "", nil
}

// getUserInfo takes a ssh url and extracts user information
func getUserInfo(uri *urlP.URL) (*urlP.Userinfo, error) {
	var (
		usr *user.User
		err error
	)
	if u, found := os.LookupEnv("_CONTAINERS_ROOTLESS_UID"); found {
		usr, err = user.LookupId(u)
		if err != nil {
			return nil, errors.Wrapf(err, "failed to lookup rootless user")
		}
	} else {
		usr, err = user.Current()
		if err != nil {
			return nil, errors.Wrapf(err, "failed to obtain current user")
		}
	}

	pw, set := uri.User.Password() // check for a password set
	if set {
		return urlP.UserPassword(usr.Username, pw), nil
	}
	return urlP.User(usr.Username), nil
}
