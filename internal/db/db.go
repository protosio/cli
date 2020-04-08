package db

import (
	"os"

	"github.com/asdine/storm"
	"github.com/pkg/errors"
	"github.com/protosio/cli/internal/cloud"
)

var dbi DB

type dbprotos struct {
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

	// generalized
	// Save writes a new value for a specific key in a bucket
	Save(data interface{}) error
	All(to interface{}) error
	Close() error
}

// Init creates a new local database used by the Protos client
func initDB(protosDir string, protosDB string) (*storm.DB, error) {
	dbPath := protosDir + protosDB

	dirInfo, err := os.Stat(protosDir)
	if err != nil {
		if os.IsNotExist(err) {
			err = os.MkdirAll(protosDir, os.FileMode(0700))
			if err != nil {
				return nil, errors.Wrapf(err, "Failed to create '%s' directory", protosDir)
			}
		} else {
			return nil, errors.Wrapf(err, "Failed to probe '%s' directory", protosDir)
		}
	} else {
		if !dirInfo.IsDir() {
			return nil, errors.Errorf("Protos path '%s' is a file, and not a directory", protosDir)
		}
	}

	db, err := storm.Open(dbPath)
	if err != nil {
		return nil, err
	}
	return db, nil

}

// Open tries to open a client for the db on the provided path
func Open(protosDir string, protosDB string) (DB, error) {
	dbPath := protosDir + protosDB
	db := &dbprotos{}
	var dbg *storm.DB

	_, err := os.Stat(dbPath)
	if err == nil {
		dbg, err = storm.Open(dbPath)
		if err != nil {
			return nil, err
		}
	} else if os.IsNotExist(err) {
		dbg, err = initDB(protosDir, protosDB)
	} else {
		return db, errors.Wrapf(err, "Failed to stat path '%s'", dbPath)
	}

	db.s = dbg
	return db, nil
}

//
// db storm methods for implementing the DB interface
//

// Save writes a new value for a specific key in a bucket
func (db *dbprotos) Save(data interface{}) error {
	return db.s.Save(data)
}

// One retrieves one record from the database based on the field name
func (db *dbprotos) One(fieldName string, value interface{}, to interface{}) error {
	return db.s.One(fieldName, value, to)
}

// All retrieves all records for a specific type
func (db *dbprotos) All(to interface{}) error {
	return db.s.All(to)
}

// Delete removes a record of specific type
func (db *dbprotos) Delete(data interface{}) error {
	return db.s.DeleteStruct(data)
}

func (db *dbprotos) SaveCloud(cloud cloud.ProviderInfo) error {
	return db.s.Save(&cloud)
}

func (db *dbprotos) DeleteCloud(name string) error {
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

func (db *dbprotos) GetCloud(name string) (cloud.ProviderInfo, error) {
	cp := cloud.ProviderInfo{}
	err := db.s.One("Name", name, &cp)
	if err != nil {
		return cp, err
	}
	return cp, nil
}

func (db *dbprotos) GetAllClouds() ([]cloud.ProviderInfo, error) {
	cps := []cloud.ProviderInfo{}
	err := db.s.All(&cps)
	if err != nil {
		return cps, err
	}
	return cps, nil
}

func (db *dbprotos) SaveInstance(instance cloud.InstanceInfo) error {
	return db.s.Save(&instance)
}

func (db *dbprotos) DeleteInstance(name string) error {
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

func (db *dbprotos) GetInstance(name string) (cloud.InstanceInfo, error) {
	instance := cloud.InstanceInfo{}
	err := db.s.One("Name", name, &instance)
	if err != nil {
		return instance, err
	}
	return instance, nil
}

func (db *dbprotos) GetAllInstances() ([]cloud.InstanceInfo, error) {
	instances := []cloud.InstanceInfo{}
	err := db.s.All(&instances)
	if err != nil {
		return instances, err
	}
	return instances, nil
}

func (db *dbprotos) Close() error {
	return db.s.Close()
}
