package ssh

import (
	"crypto/rand"
	"time"

	"github.com/pkg/errors"
	log "github.com/sirupsen/logrus"
	"golang.org/x/crypto/ed25519"
	"golang.org/x/crypto/ssh"
)

// GenerateKey generates a SSH key pair
func GenerateKey() (Key, error) {
	key := Key{}
	var err error
	key.public, key.private, err = ed25519.GenerateKey(rand.Reader)
	if err != nil {
		return key, errors.Wrap(err, "Failed to generate SSH key")
	}
	// publicKey, err := ssh.NewPublicKey(key.public)
	// if err != nil {
	// 	return key, errors.Wrap(err, "Failed to generate SSH key")
	// }

	// pemKey := &pem.Block{
	// 	Type:  "OPENSSH PRIVATE KEY",
	// 	Bytes: edkey.MarshalED25519PrivateKey(privKey),
	// }
	// privateKey := pem.EncodeToMemory(pemKey)
	// authorizedKey := ssh.MarshalAuthorizedKey(publicKey)
	return key, nil
}

// ExecuteCommand opens a session using the provided client and executes the provided command
func ExecuteCommand(cmd string, client *ssh.Client) (string, error) {
	session, err := client.NewSession()
	if err != nil {
		return "", errors.Wrap(err, "Failed to create new sessions")
	}

	modes := ssh.TerminalModes{
		ssh.ECHO:          0,
		ssh.TTY_OP_ISPEED: 14400,
		ssh.TTY_OP_OSPEED: 14400,
	}

	if err := session.RequestPty("xterm", 80, 40, modes); err != nil {
		session.Close()
		return "", errors.Wrap(err, "Request for pseudo terminal failed")
	}

	log.Infof("Executing (SSH) command '%s'", cmd)
	output, err := session.CombinedOutput(cmd)
	if err != nil {
		return string(output), errors.Wrapf(err, "Failed to execute command '%s'", cmd)
	}

	session.Close()

	return string(output), nil

}

func NewConnection(host string, user string, auth ssh.AuthMethod, maxRetries int) (*ssh.Client, error) {
	sshConfig := &ssh.ClientConfig{
		User: "root",
		Auth: []ssh.AuthMethod{
			auth,
		},
		HostKeyCallback: ssh.InsecureIgnoreHostKey(), // TODO validate server before?
	}

	tries := 0
	var client *ssh.Client
	var err error
	for {
		tries++
		if tries > maxRetries {
			return nil, errors.Wrapf(err, "Failed to open SSH connection to '%s@%s'", user, host)
		}
		client, err = ssh.Dial("tcp", host+":22", sshConfig) // TODO remove hardocoded port?
		if err != nil {
			time.Sleep(3 * time.Second)
		} else {
			break
		}
	}
	return client, nil
}
