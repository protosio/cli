package main

import "github.com/urfave/cli/v2"

var cmdRelease *cli.Command = &cli.Command{
	Name:  "releases",
	Usage: "Lists the latest available Protos releases",
	Action: func(c *cli.Context) error {
		return protosReleases()
	},
}

//
// Releases method
//

func protosReleases() error {
	return nil
}
