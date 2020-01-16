package ssh

import (
	"io"
	"net"
	"strings"

	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"
	"golang.org/x/crypto/ssh"
)

// Tunnel represents and SSH tunnel to a remote host
type Tunnel struct {
	sshHost   string
	sshUser   string
	sshAuth   ssh.AuthMethod
	sshConn   *ssh.Client
	listener  net.Listener
	localPort int
	target    string
	log       *logrus.Logger
	connMap   []chan interface{}
}

// Start initiates the ssh tunnel
func (t *Tunnel) Start() (int, error) {
	// setup the local listener using a random port
	var err error
	t.listener, err = net.Listen("tcp", "localhost:0")
	if err != nil {
		return 0, err
	}
	t.localPort = t.listener.Addr().(*net.TCPAddr).Port

	// setup the SSH connection
	sshConfig := &ssh.ClientConfig{
		User: t.sshUser,
		Auth: []ssh.AuthMethod{t.sshAuth},
		HostKeyCallback: func(hostname string, remote net.Addr, key ssh.PublicKey) error {
			// Always accept key.
			return nil
		}}
	t.sshConn, err = ssh.Dial("tcp", t.sshHost, sshConfig)
	if err != nil {
		return 0, err
	}

	// accept local connections and start the forwarding
	go func() {
		for {
			localConn, err := t.listener.Accept()
			if err != nil {
				if strings.Contains(err.Error(), "use of closed network connection") {
					t.log.Debug("Local SSH tunnel listener closed. Not accepting any new connections.")
					return
				}
				t.log.Errorf("Failed to accept connection via the SSH tunnel: %s", err)
				continue
			}
			close := make(chan interface{})
			go t.forward(localConn, t.sshConn, close)
			t.connMap = append(t.connMap, close)
		}
	}()

	return t.localPort, nil
}

// Close terminates the SSH tunnel
func (t *Tunnel) Close() error {
	// close the listener and the rest of the connections
	err := t.listener.Close()
	if err != nil {
		return errors.Wrap(err, "Error while closing local tunnel listener")
	}
	for _, close := range t.connMap {
		close <- true
	}
	err = t.sshConn.Close()
	if err != nil {
		return errors.Wrap(err, "Error while closing ssh tunnel connection")
	}

	return nil
}

func (t *Tunnel) forward(localConn net.Conn, sshConn *ssh.Client, close chan interface{}) {
	t.log.Debug("New forwarded connection")
	remoteConn, err := sshConn.Dial("tcp", t.target)
	if err != nil {
		t.log.Errorf("Failed to establish remote connection (%s) over SSH tunnel (%s): %s", t.target, t.sshHost, err)
		return
	}
	copyConn := func(writer, reader net.Conn) {
		_, err := io.Copy(writer, reader)
		if err != nil {
			t.log.Errorf("Failed to copy data over SSH tunnel (%s): %s", t.sshHost, err)
		}
	}
	go copyConn(localConn, remoteConn)
	go copyConn(remoteConn, localConn)
	<-close
	t.log.Debug("Close signal received, stopping forwarder")
	_ = localConn.Close()
	_ = remoteConn.Close()
}

// NewTunnel creates and returns an SSHTunnel
func NewTunnel(sshHost string, sshUser string, sshAuth ssh.AuthMethod, tunnelTarget string, logger *logrus.Logger) *Tunnel {
	return &Tunnel{sshHost: sshHost, sshUser: sshUser, sshAuth: sshAuth, target: tunnelTarget, log: logger}
}
