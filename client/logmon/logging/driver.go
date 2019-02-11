package logging

import (
	"fmt"
	"io"
	"log"
)

var (
	// BuiltinDrivers contains the built in registered drivers
	// which are available to use as log recollectors
	BuiltinDrivers = map[string]Factory{
		"semaas": NewSemaasDriver,
	}
)

// Driver defines an interface for user custom logging drivers
type Driver interface {
	StdErr() (io.WriteCloser, error)
	StdOut() (io.WriteCloser, error)
}

// Factory returns a new log driver from a configuration
type Factory func(map[string]interface{}, map[string]string, *log.Logger) (Driver, error)

// NewDriver is used to instantiate and return a new driver
// given the name and a logger
func NewDriver(name string, config map[string]interface{}, data map[string]string, l *log.Logger) (Driver, error) {
	// Lookup the factory function
	factory, ok := BuiltinDrivers[name]
	if !ok {
		return nil, fmt.Errorf("unknown driver '%s'", name)
	}

	// Instantiate the driver
	return factory(config, data, l)
}
