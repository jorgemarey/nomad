package api

import (
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"time"
)

// QueryOption configures how we do the query
type QueryOption func(*queryOptions)

type queryOptions struct {
	query  string
	values url.Values

	url string
}

func (q *queryOptions) setDefaults() {
	// now := time.Now()
	defaultOpts := []QueryOption{
	// QueryFrom(now.Add(-24 * time.Hour)),
	// QueryTo(now),
	// QueryPaging(100),
	// QueryOrder(OrderAsc),
	}
	for _, opt := range defaultOpts {
		opt(q)
	}
}

type searchResponse struct {
	Data       []*LogEntry
	Pagination pagination
}

type pagination struct {
	Links         links
	Page          int
	TotalPages    int
	TotalElements int
	PageSize      int
}

type links struct {
	First    string
	Last     string
	Previous string
	Next     string
}

// Search defines a function that returns some values and a pager to
// fetch more related results
type Search func() ([]*LogEntry, *Pager, error)

// Pager allow to perform paged search
type Pager struct {
	First         Search
	Last          Search
	Previous      Search
	Next          Search
	Page          int
	TotalPages    int
	TotalElements int
	PageSize      int
}

// Search performs a seach over the logs uploaded on omega
func (c *Client) Search(opts ...QueryOption) ([]*LogEntry, *Pager, error) {
	q := &queryOptions{values: url.Values{}}
	q.setDefaults()
	for _, opt := range opts {
		opt(q)
	}

	r := func() (*http.Request, error) {
		if q.url != "" {
			return c.requestCompletePath(nil, http.MethodGet, q.url, nil)
		}
		url := fmt.Sprintf("ns/%s/logs", c.Namespace())
		if qe := q.values.Encode(); qe != "" {
			url = fmt.Sprintf("%s?%s", url, qe)
		}
		return c.request(nil, http.MethodGet, url, nil)
	}
	var result searchResponse
	if err := c.do(r, &result); err != nil {
		return nil, nil, err
	}
	return result.Data, c.newPager(result.Pagination), nil
}

// NewPager returns a Pager from the result of client.Search with the
// same results as that search on the Pager paging functions
func NewPager(entries []*LogEntry, p *Pager, err error) *Pager {
	fn := func() ([]*LogEntry, *Pager, error) { return entries, p, err }
	pager := &Pager{
		First:    fn,
		Last:     fn,
		Next:     fn,
		Previous: fn,
	}
	return pager
}

func (c *Client) newPager(p pagination) *Pager {
	pager := &Pager{
		Page:          p.Page,
		TotalPages:    p.TotalPages,
		TotalElements: p.TotalElements,
		PageSize:      p.PageSize,
	}
	if p.Links.First != "" {
		pager.First = func() ([]*LogEntry, *Pager, error) { return c.Search(queryURL(p.Links.First)) }
	}
	if p.Links.Last != "" {
		pager.Last = func() ([]*LogEntry, *Pager, error) { return c.Search(queryURL(p.Links.Last)) }
	}
	if p.Links.Previous != "" {
		pager.Previous = func() ([]*LogEntry, *Pager, error) { return c.Search(queryURL(p.Links.Previous)) }
	}
	if p.Links.Next != "" {
		pager.Next = func() ([]*LogEntry, *Pager, error) { return c.Search(queryURL(p.Links.Next)) }
	}
	return pager
}

// QueryPaging set the query for a search request as paginated
func QueryPaging(pageSize int) QueryOption {
	return func(o *queryOptions) {
		o.values.Set("pageSize", strconv.Itoa(pageSize))
	}
}

// QueryPage sets a key to obtain the result of a single page when paginated
func QueryPage(key string) QueryOption {
	return func(o *queryOptions) {
		o.values.Set("paginationKey", key)
	}
}

// Order defines how to order the searchs
type Order int

// Order kinds
const (
	OrderAsc Order = iota
	OrderDesc
)

// QueryOrder sets the order for the results
func QueryOrder(order Order) QueryOption {
	orderStr := "asc"
	switch order {
	case OrderDesc:
		orderStr = "desc"
	}
	return func(o *queryOptions) {
		o.values.Set("order", orderStr)
	}
}

// TODO: orderBy

// Query sets a restQL string to perform a query
func Query(q string) QueryOption {
	return func(o *queryOptions) {
		if o.query == "" {
			o.query = q
		} else {
			o.query = fmt.Sprintf("%s AND %s", o.query, q)
		}
		o.values.Set("q", o.query)
	}
}

// QueryFrom sets an initial time for the results
func QueryFrom(from time.Time) QueryOption {
	return func(o *queryOptions) {
		query := fmt.Sprintf("creationDate>=%d", from.UnixNano())
		Query(query)(o)
	}
}

// QueryTo sets an end time for the results
func QueryTo(to time.Time) QueryOption {
	return func(o *queryOptions) {
		query := fmt.Sprintf("creationDate<=%d", to.UnixNano())
		Query(query)(o)
	}
}

// QueryFilter sets a filter on the query
func QueryFilter(param, value string) QueryOption {
	return func(o *queryOptions) {
		query := fmt.Sprintf("%s==%s", param, value)
		Query(query)(o)
	}
}

func queryURL(url string) QueryOption {
	return func(o *queryOptions) {
		o.url = url
	}
}
