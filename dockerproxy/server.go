package dockerproxy

import (
	"strings"
	"time"

	"github.com/docker/docker/client"
	"github.com/gliderlabs/ssh"
	"github.com/tkw1536/proxyssh"
	"github.com/tkw1536/proxyssh/utils"
)

// ServerOptions are options for the docker proxy server
type ServerOptions struct {
	// Client is the docker client to use for everything
	Client *client.Client
	// ListenAddress is the address to listen on
	ListenAddress string
	// DockerLabelUser is the label to find the container by
	DockerLabelUser string
	// DockerLabelAuthFile is the label to find the authorized_keys file by
	DockerLabelAuthFile string
	// DockerLabelKey is the label that may contain a set of authorized keys
	DockerLabelKey string
	// ContainerShell is the executable to run within the container
	ContainerShell string
	// DisableAuthentication disables authentication
	DisableAuthentication bool
	// IdleTimeout is the timeout after which an idle connection is killed
	IdleTimeout time.Duration
	// ForwardAddresses are addresses that port forwarding is allowed for.
	ForwardAddresses []utils.NetworkAddress
	// ReverseAddresses are addresses that reverse port forwarding is allowed for.
	ReverseAddresses []utils.NetworkAddress
}

// NewDockerProxyServer makes a new docker ssh server with the given logger and options.
// It returns a new server that was created.
func NewDockerProxyServer(logger utils.Logger, opts ServerOptions) (server *ssh.Server) {
	server = &ssh.Server{
		Handler: proxyssh.HandleShellCommand(logger, func(s ssh.Session) (command []string, err error) {
			// no commands allowed for security reasons
			command = s.Command()
			if len(command) == 0 {
				// no arguments were provided => run shell
				command = []string{opts.ContainerShell}
			} else {
				// some arguments were provided => run shell -c
				command = []string{opts.ContainerShell, "-c", strings.Join(command, " ")}
			}

			// find the container by label or bail out
			container, err := FindUniqueContainer(opts.Client, opts.DockerLabelUser, s.User())
			if err != nil {
				return nil, err
			}

			// wrap it in docker exec
			command = DockerExec(s, container.ID, command, "", "")
			return
		}),
		PublicKeyHandler: proxyssh.AuthorizeKeys(logger, func(ctx ssh.Context) ([]ssh.PublicKey, error) {
			container, err := FindUniqueContainer(opts.Client, opts.DockerLabelUser, ctx.User())
			if err != nil {
				return nil, err
			}

			keys := FindContainerKeys(opts.Client, container, DockerSSHAuthOptions{
				LabelFile: opts.DockerLabelAuthFile,
			})
			return keys, nil
		}),

		Addr:        opts.ListenAddress,
		IdleTimeout: opts.IdleTimeout,
	}

	// turn of auth when the flag is set
	if opts.DisableAuthentication {
		logger.Print("WARNING: Disabling authentication. Anyone will be able to connect. ")
		server.PublicKeyHandler = nil
	}

	server = proxyssh.AllowPortForwarding(logger, server, opts.ForwardAddresses, opts.ReverseAddresses)

	return
}
