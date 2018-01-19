package loader

import "globaldevtools.bbva.com/entsec/semaas/omega/api"

type FallbackStorer interface {
	Open(name string) (LogEntryStorer, error)
}

type LogEntryStorer interface {
	Save(entries []*api.LogEntry) error
	Load() ([]*api.LogEntry, error)
}
