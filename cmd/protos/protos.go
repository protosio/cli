package main

import (
	"fmt"
	"os"

	survey "github.com/AlecAivazis/survey/v2"
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

func protosInit() error {
	answers := struct {
		Username        string
		Name            string
		Password        string
		PasswordConfirm string
		Domain          string
	}{}

	var qs = []*survey.Question{
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
				if str, ok := val.(string); ok && str != answers.Password {
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

	// perform the questions
	err := survey.Ask(qs, &answers)
	if err != nil {
		return err
	}

	return nil
}
