package logging

import (
	"crypto/tls"
	"fmt"
	"io"
	"log"
	"os"
	"time"

	"github.com/hashicorp/nomad/client/driver/env"
	"github.com/hashicorp/nomad/nomad/structs"
	"github.com/mitchellh/mapstructure"
	"globaldevtools.bbva.com/entsec/semaas/delta/writer"
	omega "globaldevtools.bbva.com/entsec/semaas/omega/api"
	omegaLoader "globaldevtools.bbva.com/entsec/semaas/omega/loader"
	rho "globaldevtools.bbva.com/entsec/semaas/rho/api"
	rhoLoader "globaldevtools.bbva.com/entsec/semaas/rho/loader"
)

// SemaasDriver is a driver that can send logs to semaas
type SemaasDriver struct {
	cfg    *semaasDriverConfig
	logger *log.Logger
}

type semaasDriverConfig struct {
	APIKey        string              `mapstructure:"api_key"`
	Namespace     string              `mapstructure:"namespace"`
	MrID          string              `mapstructure:"mrid"`
	PropertiesRAW []map[string]string `mapstructure:"properties"`
	Properties    map[string]string   `mapstructure:"-"`
}

// NewSemaasDriver returns a new log driver that sends logs to semaas
func NewSemaasDriver(task *structs.Task, env *env.TaskEnv, l *log.Logger) (Driver, error) {
	sconf, err := newSemaasDriverConfig(task, env)
	if err != nil {
		return nil, fmt.Errorf("Unable to initialize semaas driver config: %s", err)
	}
	return &SemaasDriver{cfg: sconf, logger: l}, nil
}

func newSemaasDriverConfig(task *structs.Task, env *env.TaskEnv) (*semaasDriverConfig, error) {
	sconf := semaasDriverConfig{Properties: make(map[string]string)}

	if err := mapstructure.WeakDecode(task.LogConfig.Config, &sconf); err != nil {
		return nil, err
	}
	for _, m := range sconf.PropertiesRAW {
		for k, v := range m {
			delete(m, k)
			m[env.ReplaceEnv(k)] = env.ReplaceEnv(v)
		}
	}
	sconf.APIKey = env.ReplaceEnv(sconf.APIKey)
	sconf.MrID = env.ReplaceEnv(sconf.MrID)
	sconf.Namespace = env.ReplaceEnv(sconf.Namespace)
	sconf.Properties = mapMergeStrStr(sconf.PropertiesRAW...)

	properties := map[string]string{
		"allocation":  env.EnvMap["NOMAD_ALLOC_ID"],
		"alloc_index": env.EnvMap["NOMAD_ALLOC_INDEX"],
		"task":        env.EnvMap["NOMAD_TASK_NAME"],
		"group":       env.EnvMap["NOMAD_GROUP_NAME"],
		"job":         env.EnvMap["NOMAD_JOB_NAME"],
		"dc":          env.EnvMap["NOMAD_DC"],
		"region":      env.EnvMap["NOMAD_REGION"],
	}
	for k, v := range properties {
		sconf.Properties[k] = v
	}

	return &sconf, nil
}

func newSemaasLoader(conf *semaasDriverConfig, logger *log.Logger) (*omegaLoader.Loader, *rhoLoader.Loader, error) {
	cert, err := tls.LoadX509KeyPair("/etc/certs/semaas.crt", "/etc/certs/semaas.key")
	if err != nil {
		return nil, nil, fmt.Errorf("Can't load certificate: %s", err)
	}
	oc, err := omega.NewClient(
		omega.WithClientCert(cert),
		omega.WithNamespace(conf.Namespace),
		omega.WithURL(os.Getenv("OMEGA_URL")),
		omega.WithSkipVerify(),
		omega.WithSystemLogger(logger),
	)
	if err != nil {
		return nil, nil, fmt.Errorf("Error creating omega client: %s", err)
	}
	logLoader, _ := omegaLoader.Bulk(
		oc,
		omegaLoader.WithTimeout(5*time.Second),
		omegaLoader.WithBulkSize(512, 500),
		omegaLoader.WithSafeThreshold(1024),
		omegaLoader.WithSystemLogger(logger),
	)
	rc, err := rho.NewClient(
		rho.WithClientCert(cert),
		rho.WithNamespace(conf.Namespace),
		rho.WithURL(os.Getenv("RHO_URL")),
		rho.WithSkipVerify(),
		rho.WithSystemLogger(logger),
	)
	if err != nil {
		return nil, nil, fmt.Errorf("Error creating rho client: %s", err)
	}
	traceLoader, _ := rhoLoader.Bulk(
		rc,
		rhoLoader.WithTimeout(5*time.Second),
		rhoLoader.WithBulkSize(512, 500),
		rhoLoader.WithSafeThreshold(1024),
		rhoLoader.WithSystemLogger(logger),
	)
	return logLoader, traceLoader, nil
}

func mapMergeStrStr(maps ...map[string]string) map[string]string {
	out := map[string]string{}
	for _, in := range maps {
		for key, val := range in {
			out[key] = val
		}
	}
	return out
}

func (d *SemaasDriver) stdStream(kind string) (io.WriteCloser, error) {
	ll, tl, _ := newSemaasLoader(d.cfg, d.logger)
	p := make(map[string]interface{}, len(d.cfg.Properties))
	for k, v := range d.cfg.Properties {
		p[k] = v
	}
	p["stream"] = kind
	d.logger.Printf("[INFO] logging: creating new %s logger with namespace:%s mrID:%s", kind, d.cfg.Namespace, d.cfg.MrID)
	return writer.New(ll, tl, omega.LogLevelInfo, p, d.cfg.MrID, d.logger), nil
}

// StdErr returns the WriteCloser for the standart error stream
func (d *SemaasDriver) StdErr() (io.WriteCloser, error) {
	return d.stdStream("stderr")
}

// StdOut returns the WriteCloser for the standart output stream
func (d *SemaasDriver) StdOut() (io.WriteCloser, error) {
	return d.stdStream("stdout")
}
