package ssh

import (
	"crypto/rand"
	"encoding/pem"

	"github.com/mikesmitty/edkey"
	"github.com/pkg/errors"
	log "github.com/sirupsen/logrus"
	"golang.org/x/crypto/ed25519"
	"golang.org/x/crypto/ssh"
)

// GenerateKey generates a SSH key pair
func GenerateKey() ([]byte, string, error) {
	pubKey, privKey, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		return nil, "", errors.Wrap(err, "Failed to generate SSH key")
	}
	publicKey, err := ssh.NewPublicKey(pubKey)
	if err != nil {
		return nil, "", errors.Wrap(err, "Failed to generate SSH key")
	}

	pemKey := &pem.Block{
		Type:  "OPENSSH PRIVATE KEY",
		Bytes: edkey.MarshalED25519PrivateKey(privKey),
	}
	privateKey := pem.EncodeToMemory(pemKey)
	authorizedKey := ssh.MarshalAuthorizedKey(publicKey)
	return privateKey, string(authorizedKey), nil
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
