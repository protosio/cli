package main

import (
	"fmt"

	"github.com/protosio/cli/internal/user"
	"github.com/urfave/cli/v2"
)

var cmdUser *cli.Command = &cli.Command{
	Name:  "user",
	Usage: "Manage local user details",
	Subcommands: []*cli.Command{
		{
			Name:  "info",
			Usage: "Prints info about the local user configured during init",
			Action: func(c *cli.Context) error {
				return infoUser()
			},
		},
	},
}

func infoUser() error {
	user, err := user.Get(envi)
	if err != nil {
		return err
	}
	fmt.Printf("Username: %s\n", user.Username)
	fmt.Printf("Name: %s\n", user.Name)
	fmt.Printf("Domain: %s\n", user.Domain)
	return nil
}
