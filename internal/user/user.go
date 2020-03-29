package user

import (
	"errors"
	"fmt"

	"github.com/protosio/cli/internal/env"
)

// Info represents the local Protos user
type Info struct {
	env      *env.Env
	Username string `storm:"id"`
	Name     string
	Domain   string
}

// Save saves the user to db
func (ui Info) save() {
	err := ui.env.DB.Save(&ui)
	if err != nil {
		panic(err)
	}
}

// SetName enables the changing of the name of the user
func (ui Info) SetName(name string) error {
	ui.Name = name
	ui.save()
	return nil
}

// SetDomain enables the changing of the domain of the user
func (ui Info) SetDomain(domain string) error {
	ui.Domain = domain
	ui.save()
	return nil
}

// New creates and returns a new user. Also validates the data
func New(env *env.Env, username string, name string, domain string) (Info, error) {
	usrInfo, err := Get(env)
	if err == nil {
		return usrInfo, fmt.Errorf("User '%s' already initialized. Modify it using the 'user set' command", usrInfo.Username)
	}
	user := Info{env: env, Username: username, Name: name, Domain: domain}
	user.save()
	return user, nil
}

// Get returns information about the local user
func Get(env *env.Env) (Info, error) {
	users := []Info{}
	err := env.DB.All(&users)
	if err != nil {
		panic(err)
	}
	if len(users) < 1 {
		return Info{}, errors.New("There is no user info")
	} else if len(users) > 1 {
		panic("Found more than one user, please delete DB and re-run init")
	}
	usr := users[0]
	usr.env = env
	return usr, nil
}
