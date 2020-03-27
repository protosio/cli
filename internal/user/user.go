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
func (ui Info) Save() {
	err := ui.env.DB.Save(&ui)
	if err != nil {
		panic(err)
	}
}

// New creates and returns a new user. Also validates the data
func New(env *env.Env, username string, name string, domain string) (Info, error) {
	usrInfo, err := Get(env)
	if err == nil {
		return usrInfo, fmt.Errorf("User '%s' already initialized", usrInfo.Username)
	}
	user := Info{env: env, Username: username, Name: name, Domain: domain}
	user.Save()
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
		return Info{}, errors.New("Please run init first as there is no user info")
	} else if len(users) > 1 {
		panic("Found more than one user, please delete DB and re-run init")
	}
	return users[0], nil
}
