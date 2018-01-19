package api

import (
	"fmt"
	"net/http"
)

// Load sends a bulk of logs entries to omega
func (c *Client) Load(entries []*LogEntry) error { // THINK: change to variadic?
	url := fmt.Sprintf("ns/%s/logs", c.Namespace())
	r := func() (*http.Request, error) { return c.request(nil, http.MethodPost, url, entries) }
	return c.do(r, nil)
}
