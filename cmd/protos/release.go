package main

import (
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"text/tabwriter"
	"time"

	"github.com/pkg/errors"
	"github.com/urfave/cli/v2"
)

const (
	releasesURL = "https://releases.protos.io/releases.json"
)

var cmdRelease *cli.Command = &cli.Command{
	Name:  "release",
	Usage: "Lists the latest available Protos releases",
	Action: func(c *cli.Context) error {
		_, err := protosRelease()
		return err
	},
}

type CloudImage struct {
	Provider    string
	URL         string
	Digest      string
	ReleaseDate time.Time `json:"release-date"`
}

type Release struct {
	CloudImages map[string]CloudImage `json:"cloud-images"`
	Version     string
	Description string
	ReleaseDate time.Time `json:"release-date"`
}

type Releases struct {
	Releases map[string]Release
}

//
// Releases methods
//

func protosRelease() (Releases, error) {
	var releases Releases
	resp, err := http.Get(releasesURL)
	if err != nil {
		return releases, errors.Wrapf(err, "Failed to retrieve releases from '%s'", releasesURL)
	}
	defer resp.Body.Close()

	err = json.NewDecoder(resp.Body).Decode(&releases)
	if err != nil {
		return releases, errors.Wrap(err, "Failed to JSON decode the releases response")
	}

	w := new(tabwriter.Writer)
	w.Init(os.Stdout, 0, 0, 2, ' ', 0)

	defer w.Flush()

	fmt.Fprintf(w, " %s\t%s\t%s\t", "Version", "Date", "Description")
	fmt.Fprintf(w, "\n %s\t%s\t%s\t", "-------", "----", "-----------")
	for _, release := range releases.Releases {
		fmt.Fprintf(w, "\n %s\t%s\t%s\t", release.Version, release.ReleaseDate.Format("Jan 2, 2006"), release.Description)
	}
	fmt.Fprint(w, "\n")

	return releases, nil
}
