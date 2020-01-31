package main

import (
	"os"

	"github.com/protosio/cli/internal/db"
	"github.com/sirupsen/logrus"
	"github.com/urfave/cli/v2"
)

var log *logrus.Logger
var dbp db.DB
var cloudName string
var cloudLocation string
var protosVersion string

func main() {
	log = logrus.New()
	var loglevel string
	app := &cli.App{
		Name:    "protos-cli",
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
			cmdInit,
			cmdRelease,
			cmdCloud,
			cmdInstance,
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

	app.After = func(c *cli.Context) error {
		if dbp != nil {
			return dbp.Close()
		}
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
