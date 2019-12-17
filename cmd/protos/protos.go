package main

import (
	"fmt"
	"os"

	survey "github.com/AlecAivazis/survey/v2"
	"github.com/protosio/cli/internal/cloud"
	log "github.com/sirupsen/logrus"
	"github.com/urfave/cli/v2"
)

func main() {
	app := &cli.App{
		Name:  "init",
		Usage: "Initializes Protos locally and deploys an instance in one of the supported clouds",
		Action: func(c *cli.Context) error {
			return protosInit()
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

func protosInit() error {

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

	err = client.Init(transformCredentials(cloudCredentials))
	if err != nil {
		return err
	}

	// get user details
	userDetails := userDetails{}
	userDetailsQuestions := getUserDetailsQuestions(&userDetails)
	err = survey.Ask(userDetailsQuestions, &userDetails)
	if err != nil {
		return err
	}

	return nil
}
