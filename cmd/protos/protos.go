package main

import (
	"fmt"
	"os"
	"text/tabwriter"

	survey "github.com/AlecAivazis/survey/v2"
	"github.com/pkg/errors"
	"github.com/protosio/cli/internal/cloud"
	"github.com/protosio/cli/internal/db"
	"github.com/sirupsen/logrus"
	"github.com/urfave/cli/v2"
)

var log *logrus.Logger
var dbp db.DB

func main() {
	log = logrus.New()
	var loglevel string
	app := &cli.App{
		Name:    "protos",
		Usage:   "Command-line client for Protos",
		Version: "0.0.0-dev",
		Flags: []cli.Flag{
			&cli.StringFlag{
				Name:        "log, l",
				Value:       "info",
				Usage:       "Log level: warn, info, debug",
				Destination: &loglevel,
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
			{
				Name:  "cloud",
				Usage: "Manage clouds",
				Subcommands: []*cli.Command{
					{
						Name:  "ls",
						Usage: "List existing cloud provider accounts",
						Action: func(c *cli.Context) error {
							return listCloudProviders()
						},
					},
					{
						Name:      "add",
						ArgsUsage: "<name>",
						Usage:     "Add a new cloud provider account",
						Action: func(c *cli.Context) error {
							name := c.Args().Get(0)
							if name == "" {
								cli.ShowSubcommandHelp(c)
								os.Exit(1)
							}
							_, err := addCloudProvider(name)
							return err
						},
					},
					{
						Name:      "delete",
						ArgsUsage: "<name>",
						Usage:     "Delete an existing cloud provider account",
						Action: func(c *cli.Context) error {
							name := c.Args().Get(0)
							if name == "" {
								cli.ShowSubcommandHelp(c)
								os.Exit(1)
							}
							return deleteCloudProvider(name)
						},
					},
					{
						Name:      "check",
						ArgsUsage: "<name>",
						Usage:     "Checks validity of an existing cloud provider account",
						Action: func(c *cli.Context) error {
							name := c.Args().Get(0)
							if name == "" {
								cli.ShowSubcommandHelp(c)
								os.Exit(1)
							}
							return checkCloudProvider(name)
						},
					},
				},
			},
		},
	}

	app.Before = func(c *cli.Context) error {
		level, err := logrus.ParseLevel(loglevel)
		if err != nil {
			return err
		}
		log.SetLevel(level)
		config(c.Args().First())
		return nil
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

func config(currentCmd string) {
	var err error
	if currentCmd != "init" {
		dbp, err = db.Open("")
		if err != nil {
			log.Fatal(err)
		}
	}
}

func listCloudProviders() error {
	clouds, err := dbp.GetAllClouds()
	if err != nil {
		return err
	}

	w := new(tabwriter.Writer)
	w.Init(os.Stdout, 16, 16, 0, '\t', 0)

	defer w.Flush()

	fmt.Fprintf(w, " %s\t%s\t", "Name", "Type")
	fmt.Fprintf(w, "\n %s\t%s\t", "----", "----")
	for _, cl := range clouds {
		fmt.Fprintf(w, "\n %s\t%s\t", cl.Name, cl.Type)
	}
	fmt.Fprint(w, "\n")
	return nil
}

func addCloudProvider(cloudName string) (cloud.Provider, error) {
	// select cloud provider
	var cloudType string
	cloudProviderSelect := surveySelect(cloud.SupportedProviders(), "Choose one of the following supported cloud providers:")
	err := survey.AskOne(cloudProviderSelect, &cloudType)
	if err != nil {
		return nil, err
	}

	// create new cloud provider
	client, err := cloud.NewProvider(cloudName, cloudType)
	if err != nil {
		return nil, err
	}

	// get cloud provider credentials
	cloudCredentials := map[string]interface{}{}
	credFields := client.AuthFields()
	credentialsQuestions := getCloudCredentialsQuestions(cloudType, credFields)

	err = survey.Ask(credentialsQuestions, &cloudCredentials)
	if err != nil {
		return nil, err
	}

	// get cloud provider location
	var cloudLocation string
	supportedLocations := client.SupportedLocations()
	cloudLocationQuestions := surveySelect(supportedLocations, fmt.Sprintf("Choose one of the following supported locations supported for '%s':", cloudType))
	err = survey.AskOne(cloudLocationQuestions, &cloudLocation)
	if err != nil {
		return nil, err
	}

	// init cloud client
	err = client.Init(transformCredentials(cloudCredentials), cloudLocation)
	if err != nil {
		return nil, err
	}

	// save the cloud provider in the db
	cloudProviderInfo := client.GetInfo()
	err = dbp.SaveCloud(cloudProviderInfo)
	if err != nil {
		return nil, errors.Wrap(err, "Failed to save cloud provider info")
	}

	return client, nil
}

func deleteCloudProvider(name string) error {
	return dbp.DeleteCloud(name)
}

func checkCloudProvider(name string) error {
	cloud, err := dbp.GetCloud(name)
	if err != nil {
		return errors.Wrapf(err, "Could not retrieve cloud '%s'", name)
	}
	client := cloud.Client()
	locations := client.SupportedLocations()
	err = client.Init(cloud.Auth, locations[0])
	if err != nil {
		return errors.Wrapf(err, "Failed to connect to cloud provider '%s'(%s) API", name, cloud.Type.String())
	}
	fmt.Printf("Name: %s\n", cloud.Name)
	fmt.Printf("Type: %s\n", cloud.Type.String())
	if err != nil {
		fmt.Printf("Status: NOT OK (%s)\n", err.Error())
	} else {
		fmt.Printf("Status: OK\n")
	}
	return nil
}
