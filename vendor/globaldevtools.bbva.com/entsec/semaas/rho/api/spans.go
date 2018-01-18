package api

import (
	"fmt"
	"net/http"
)

// Span represents a contiguous segment of work in a Trace
type Span struct {
	MrID       string                 `json:"mrId"`
	Name       string                 `json:"name,omitempty"`
	SpanID     string                 `json:"spanId,omitempty"`
	TraceID    string                 `json:"traceId,omitempty"`
	ParentSpan string                 `json:"parentSpan,omitempty"`
	StartDate  int64                  `json:"startDate,omitempty"`
	FinishDate int64                  `json:"finishDate,omitempty"`
	Duration   int64                  `json:"duration,omitempty"`
	RecordDate int64                  `json:"recordDate,omitempty"`
	Properties map[string]interface{} `json:"properties,omitempty"`
}

// Create sends a bulk of spans to tho
func (c *Client) Create(spans []*Span) error { // THINK: change to variadic?
	url := fmt.Sprintf("ns/%s/spans", c.Namespace())
	r := func() (*http.Request, error) { return c.request(nil, http.MethodPost, url, spans) }
	return c.do(r, nil)
}
