package loader

import "globaldevtools.bbva.com/entsec/semaas/rho/api"

type FallbackStorer interface {
	Open(name string) (SpanStorer, error)
}

type SpanStorer interface {
	Save(entries []*api.Span) error
	Load() ([]*api.Span, error)
}
