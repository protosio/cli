package main

import (
	"fmt"

	survey "github.com/AlecAivazis/survey/v2"
	"github.com/pkg/errors"
	"github.com/protosio/cli/internal/db"
	ssh "github.com/protosio/cli/internal/ssh"
)

func protosDBInit() error {
	// create Protos DB
	log.Info("Initializing DB")
	dbPath, err := db.Init()
	if err != nil {
		return errors.Wrap(err, "Failed to initialize Protos DB")
	}
	dbp, err = db.Open(dbPath)
	if err != nil {
		return err
	}
	return nil
}

func protosFullInit() error {

	// create Protos DB
	dbPath, err := db.Init()
	if err != nil {
		return errors.Wrap(err, "Failed to initialize Protos")
	}
	dbp, err = db.Open(dbPath)
	if err != nil {
		return err
	}

	//
	// add cloud provider
	//

	// get a name to use internally for this specific cloud provider + credentials. This allows for adding multiple accounts of the same cloud
	cloudNameQuestion := []*survey.Question{{
		Name:     "name",
		Prompt:   &survey.Input{Message: "In the following step you will add a cloud provider. Write a name used to identify this cloud provider account internally:"},
		Validate: survey.Required,
	}}
	var cloudName string
	err = survey.Ask(cloudNameQuestion, &cloudName)

	_, err = addCloudProvider(cloudName)
	if err != nil {
		return err
	}

	//
	// Protos instance creation steps
	//

	// get a name to use internally for this specific cloud provider + credentials. This allows for adding multiple accounts of the same cloud
	vmNameQuestion := []*survey.Question{{
		Name:     "name",
		Prompt:   &survey.Input{Message: "Write a name used to identify Protos instance that will be deployed next:"},
		Validate: survey.Required,
	}}
	var vmName string
	err = survey.Ask(vmNameQuestion, &vmName)

	instanceInfo, err := addInstance(vmName, cloudName)
	if err != nil {
		return errors.Wrap(err, "Failed to initialize Protos")
	}

	//
	// Perform setup via SSH tunnel
	//

	key, err := ssh.NewKeyFromSeed(instanceInfo.KeySeed)
	if err != nil {
		return errors.Wrap(err, "Failed to initialize Protos")
	}

	// test SSH and create SSH tunnel used for initialisation
	tempClient, err := ssh.NewConnection(instanceInfo.PublicIP, "root", key.SSHAuth(), 10)
	if err != nil {
		return errors.Wrap(err, "Failed to connect to Protos instance via SSH")
	}
	tempClient.Close()
	log.Info("Instance is ready and accepting SSH connections. Perform instance setup using the web based dashboard")

	// create tunnel to reach the instance dashboard
	tunnelInstance(instanceInfo.Name)
	log.Infof("Protos instance '%s' - '%s' deployed successfully", vmName, instanceInfo.PublicIP)

	return nil
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

func surveySelect(options []string, message string) *survey.Select {
	return &survey.Select{
		Message: message,
		Options: options,
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
