package db

import (
	"os"
	"os/user"

	"github.com/asdine/storm"
	"github.com/pkg/errors"
	"github.com/protosio/cli/internal/cloud"
)

const (
	// DefaultPath indicates the default path where the DB file is saved
	DefaultPath = "/.protos/protos.db"
)

type dbstorm struct {
	s *storm.DB
}

// DB represents a DB client instance, used to interract with the database
type DB interface {
	SaveCloud(cloud cloud.ProviderInfo) error
	DeleteCloud(name string) error
	GetCloud(name string) (cloud.ProviderInfo, error)
	GetAllClouds() ([]cloud.ProviderInfo, error)
	SaveInstance(instance cloud.InstanceInfo) error
	DeleteInstance(name string) error
	GetInstance(name string) (cloud.InstanceInfo, error)
	GetAllInstances() ([]cloud.InstanceInfo, error)
	Close() error
}

// Init creates a new local database used by the Protos client
func Init() (string, error) {
	usr, _ := user.Current()
	protosDir := usr.HomeDir + "/.protos"
	protosDB := protosDir + "/protos.db"
	_, err := os.Stat(protosDB)
	if err == nil {
		return protosDB, errors.Errorf("A file exists on path '%s'. Remove it and start the init process again", protosDB)
	} else if !os.IsNotExist(err) {
		return protosDB, errors.Wrapf(err, "Failed to stat path '%s'", protosDB)
	}

	dirInfo, err := os.Stat(protosDir)
	if err != nil {
		if os.IsNotExist(err) {
			err = os.MkdirAll(protosDir, os.FileMode(0700))
			if err != nil {
				return protosDB, errors.Wrapf(err, "Failed to create '%s' directory", protosDir)
			}
		} else {
			return protosDB, errors.Wrapf(err, "Failed to probe '%s' directory", protosDir)
		}
	} else {
		if !dirInfo.IsDir() {
			return protosDB, errors.Errorf("Protos path '%s' is a file, and not a directory", protosDir)
		}
	}

	return protosDB, New(protosDB)
}

// New create a new DB at the path specified
func New(path string) error {
	db, err := storm.Open(path)
	if err != nil {
		return err
	}
	defer db.Close()
	return nil
}

// Open tries to open a client for the db on the provided path
func Open(path string) (DB, error) {
	if path == "" {
		usr, _ := user.Current()
		path = usr.HomeDir + DefaultPath
	}
	_, err := os.Stat(path)
	if err != nil {
		return nil, errors.Wrap(err, "Can't find database file. Please run init")
	}
	db := &dbstorm{}
	dbg, err := storm.Open(path)
	if err != nil {
		return nil, err
	}
	db.s = dbg
	return db, nil
}

//
// db storm methods for implementing the DB interface
//

func (db *dbstorm) SaveCloud(cloud cloud.ProviderInfo) error {
	return db.s.Save(&cloud)
}

func (db *dbstorm) DeleteCloud(name string) error {
	cp := cloud.ProviderInfo{}
	err := db.s.One("Name", name, &cp)
	if err != nil {
		return err
	}

	err = db.s.Delete("ProviderInfo", name)
	if err != nil {
		return err
	}
	return nil
}

func (db *dbstorm) GetCloud(name string) (cloud.ProviderInfo, error) {
	cp := cloud.ProviderInfo{}
	err := db.s.One("Name", name, &cp)
	if err != nil {
		return cp, err
	}
	return cp, nil
}

func (db *dbstorm) GetAllClouds() ([]cloud.ProviderInfo, error) {
	cps := []cloud.ProviderInfo{}
	err := db.s.All(&cps)
	if err != nil {
		return cps, err
	}
	return cps, nil
}

func (db *dbstorm) SaveInstance(instance cloud.InstanceInfo) error {
	return db.s.Save(&instance)
}

func (db *dbstorm) DeleteInstance(name string) error {
	instance := cloud.InstanceInfo{}
	err := db.s.One("Name", name, &instance)
	if err != nil {
		return err
	}

	err = db.s.Delete("InstanceInfo", name)
	if err != nil {
		return err
	}
	return nil
}

func (db *dbstorm) GetInstance(name string) (cloud.InstanceInfo, error) {
	instance := cloud.InstanceInfo{}
	err := db.s.One("Name", name, &instance)
	if err != nil {
		return instance, err
	}
	return instance, nil
}

func (db *dbstorm) GetAllInstances() ([]cloud.InstanceInfo, error) {
	instances := []cloud.InstanceInfo{}
	err := db.s.All(&instances)
	if err != nil {
		return instances, err
	}
	return instances, nil
}

func (db *dbstorm) Close() error {
	return db.s.Close()
}
