package sentinel

import "time"

type Sentinel struct {
	evalTimeout time.Duration
}

type Config struct {
	// EvalTimeout is the timeout for a single policy execution.
	// This must be set to some non-zero value or it will default to
	// 1 second.
	EvalTimeout time.Duration
}

func New(cfg *Config) *Sentinel {
	// ctx, ctxCancel := context.WithCancel(context.Background())

	s := &Sentinel{
		evalTimeout: 30 * time.Second,
		// cancelFunc:  ctxCancel,
	}

	if cfg != nil {
		s.evalTimeout = cfg.EvalTimeout
	}
	return s
}
