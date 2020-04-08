package env

import (
	"github.com/protosio/cli/internal/db"
	"github.com/sirupsen/logrus"
)

// Env is a struct that containts program dependencies that get injected in other modules
type Env struct {
	DB  db.DB
	Log *logrus.Logger
}

// New creates and returns an instance of Env
func New(db db.DB, log *logrus.Logger) *Env {

	if db == nil || log == nil {
		panic("env: db || log should not be nil")
	}
	return &Env{DB: db, Log: log}
}
