package main

import (
	"fmt"
	"log"
	"os"
	"os/signal"
	"os/user"
	"syscall"

	survey "github.com/AlecAivazis/survey/v2"
	"github.com/pkg/errors"
	"github.com/protosio/cli/internal/cloud"
	ssh "github.com/protosio/cli/internal/ssh"
	"github.com/sirupsen/logrus"
	"github.com/urfave/cli/v2"
)

func main() {
	app := &cli.App{
		Name:    "protos",
		Usage:   "Command-line client for Protos",
		Version: "0.0.0-dev",
		Flags: []cli.Flag{
			&cli.StringFlag{
				Name:  "log, l",
				Value: "info",
				Usage: "Log level: warn, info, debug",
			},
		},
		Commands: []*cli.Command{
			{
				Name:  "init",
				Usage: "Initializes Protos locally and deploys an instance in one of the supported clouds",
				Action: func(c *cli.Context) error {
					return protosInit()
				},
			},
		},
	}

	err := app.Run(os.Args)
	if err != nil {
		log.Fatal(err)
	}
}

type userDetails struct {
	Username        string
	Name            string
	Password        string
	PasswordConfirm string
	Domain          string
}

func getUserDetailsQuestions(ud *userDetails) []*survey.Question {
	return []*survey.Question{
		{
			Name:      "username",
			Prompt:    &survey.Input{Message: "Username:"},
			Validate:  survey.Required,
			Transform: survey.ToLower,
		},
		{
			Name:      "name",
			Prompt:    &survey.Input{Message: "Name:"},
			Validate:  survey.Required,
			Transform: survey.Title,
		},
		{
			Name:     "password",
			Prompt:   &survey.Password{Message: "Password:"},
			Validate: survey.Required,
		},
		{
			Name:   "passwordconfirm",
			Prompt: &survey.Password{Message: "Confirm password:"},
			Validate: func(val interface{}) error {
				if str, ok := val.(string); ok && str != ud.Password {
					return fmt.Errorf("passwords don't match")
				}
				return nil
			},
		},
		{
			Name:     "domain",
			Prompt:   &survey.Input{Message: "Domain name (registered with one of the supported domain providers)"},
			Validate: survey.Required,
		},
	}
}

func getCloudProviderSelect(cloudProviders []string) *survey.Select {
	return &survey.Select{
		Message: "Choose one of the following supported cloud providers:",
		Options: cloudProviders,
	}
}

func getCloudCredentialsQuestions(providerName string, fields []string) []*survey.Question {
	qs := []*survey.Question{}
	for _, field := range fields {
		qs = append(qs, &survey.Question{
			Name:     field,
			Prompt:   &survey.Input{Message: providerName + " " + field + ":"},
			Validate: survey.Required})
	}
	return qs
}

func transformCredentials(creds map[string]interface{}) map[string]string {
	transformed := map[string]string{}
	for name, val := range creds {
		transformed[name] = val.(string)
	}
	return transformed
}

func catchSignals(sigs chan os.Signal, quit chan interface{}) {
	<-sigs
	quit <- true
}

func protosInit() error {

	log := logrus.New()
	log.SetLevel(logrus.DebugLevel)

	// create config and state directory
	usr, _ := user.Current()
	protosDir := usr.HomeDir + "/.protos"
	log.Infof("Creating Protos directory '%s'", protosDir)
	err := os.MkdirAll(protosDir, os.FileMode(0600))
	if err != nil {
		return errors.Wrap(err, "Failed to initialize Protos")
	}

	// select cloud provider
	var cloudProvider string
	cloudProviders := cloud.SupportedProviders()
	cloudProviderSelect := getCloudProviderSelect(cloudProviders)

	survey.AskOne(cloudProviderSelect, &cloudProvider)

	// get cloud provider credentials
	client, err := cloud.NewClient(cloudProvider)

	credFields := client.AuthFields()

	credentialsQuestions := getCloudCredentialsQuestions(cloudProvider, credFields)
	cloudCredentials := map[string]interface{}{}

	err = survey.Ask(credentialsQuestions, &cloudCredentials)
	if err != nil {
		return err
	}

	// init cloud client
	err = client.Init(transformCredentials(cloudCredentials))
	if err != nil {
		return err
	}

	imageID := ""
	images, err := client.GetImages()
	if err != nil {
		return errors.Wrap(err, "Failed to initialize Protos")
	}
	if id, found := images["protos-image"]; found == true {
		log.Infof("Found latest Protos image (%s) in your infra cloud account", id)
		imageID = id
	} else {
		// upload protos image
		log.Info("Latest Protos image not in your infra cloud account. Adding it.")
		imageID, err = client.AddImage("https://releases.protos.io/test/scaleway-efi.iso", "4b49901e65420b55170d95768f431595")
		if err != nil {
			return errors.Wrap(err, "Failed to initialize Protos")
		}
	}

	// create SSH key used for instance
	log.Info("Generating SSH key for the new VM instance")
	key, err := ssh.GenerateKey()
	if err != nil {
		return errors.Wrap(err, "Failed to initialize Protos")
	}

	// create protos data volume

	// deploy a protos instance
	vmName := "protos1"
	log.Infof("Deploying Protos instance '%s' using image '%s'", vmName, imageID)
	vmIP, err := client.NewInstance(vmName, imageID, key.Public())
	if err != nil {
		return errors.Wrap(err, "Failed to deploy Protos instance")
	}

	// log.Info(key.EncodePrivateKeytoPEM())

	// test SSH and create SSH tunnel used for initialisation
	tempClient, err := ssh.NewConnection(vmIP, "root", key.SSHAuth(), 10)
	if err != nil {
		return errors.Wrap(err, "Failed to initialize Protos")
	}
	tempClient.Close()
	log.Info("Instance is ready")

	log.Infof("Creating SSH tunnel to the new VM, using ip '%s'", vmIP)
	tunnel := ssh.NewTunnel(vmIP+":22", "root", key.SSHAuth(), "localhost:8080", log)
	localPort, err := tunnel.Start()
	if err != nil {
		return errors.Wrap(err, "Failed to initialize Protos")
	}
	log.Infof("Please do the setup using a browser at 'http://localhost:%d/'. Once finished, press CTRL+C to continue", localPort)

	quit := make(chan interface{}, 1)
	sigs := make(chan os.Signal, 1)
	signal.Notify(sigs, syscall.SIGINT, syscall.SIGTERM)
	go catchSignals(sigs, quit)
	<-quit

	log.Info("CTRL+C received. Shutting down SSH tunnel")
	err = tunnel.Close()
	if err != nil {
		return errors.Wrap(err, "Failed to initialize Protos")
	}
	log.Info("SSH tunnel terminated successfully")
	log.Infof("Protos instance '%s' - '%s' deployed successfully", vmName, vmIP)

	// // get user details
	// userDetails := userDetails{}
	// userDetailsQuestions := getUserDetailsQuestions(&userDetails)
	// err = survey.Ask(userDetailsQuestions, &userDetails)
	// if err != nil {
	// 	return err
	// }

	return nil
}
