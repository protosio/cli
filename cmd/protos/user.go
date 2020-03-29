package main

import (
	"fmt"
	"os"

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
		{
			Name:  "set",
			Usage: "Allows you to modify details about the user (name and domain supported)",
			Subcommands: []*cli.Command{
				{
					Name:  "name",
					Usage: "Set new `NAME` for the user",
					Action: func(c *cli.Context) error {
						name := c.Args().Get(0)
						if name == "" {
							cli.ShowSubcommandHelp(c)
							os.Exit(1)
						}
						usr, err := user.Get(envi)
						if err != nil {
							return err
						}
						return usr.SetName(name)
					},
				},
				{
					Name:  "domain",
					Usage: "Set new `DOMAIN` for the user",
					Action: func(c *cli.Context) error {
						domain := c.Args().Get(0)
						if domain == "" {
							cli.ShowSubcommandHelp(c)
							os.Exit(1)
						}
						usr, err := user.Get(envi)
						if err != nil {
							return err
						}
						return usr.SetDomain(domain)
					},
				},
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
