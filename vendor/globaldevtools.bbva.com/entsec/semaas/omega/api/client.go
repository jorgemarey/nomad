package api

import (
	"bytes"
	"context"
	"crypto/tls"
	"encoding/json"
	"fmt"
	"io"
	"io/ioutil"
	"log"
	"net"
	"net/http"
	"net/http/httputil"
	"os"
	"strings"
	"time"
)

// Api Object structs

const (
	envNamespace = "OMEGA_NAMESPACE"
	envURL       = "OMEGA_URL"
	envAPIKey    = "OMEGA_API_KEY"
)

// Api logic

type clientOptions struct {
	url       string
	namespace string
	apiKey    string
	debug     bool
	tlsConfig tls.Config
	sysLogger *log.Logger
}

// ClientOption sets options over the client
type ClientOption func(*clientOptions)

// WithDebug sets the Client to debug the requests and responses
func WithDebug() ClientOption {
	return func(o *clientOptions) {
		o.debug = true
	}
}

// WithURL sets the URL where to make the load requests
func WithURL(URL string) ClientOption {
	return func(o *clientOptions) {
		o.url = URL
	}
}

// WithNamespace sets the namespace making the requests
func WithNamespace(namespace string) ClientOption {
	return func(o *clientOptions) {
		o.namespace = namespace
	}
}

// WithAPIKey sets the Api-Key to make authenticated requests
func WithAPIKey(APIKey string) ClientOption {
	return func(o *clientOptions) {
		o.apiKey = APIKey
	}
}

// WithSkipVerify skips TLS verification when comunicating with the server
func WithSkipVerify() ClientOption {
	return func(o *clientOptions) {
		o.tlsConfig.InsecureSkipVerify = true
	}
}

// WithClientCert set a client certificate to perform authentication.
// Can be read of a file using tls.LoadX509KeyPair()
func WithClientCert(cert tls.Certificate) ClientOption {
	return func(o *clientOptions) {
		o.tlsConfig.Certificates = []tls.Certificate{cert}
	}
}

func WithSystemLogger(l *log.Logger) ClientOption {
	return func(o *clientOptions) {
		o.sysLogger = l
	}
}

// Client represent a client to the omega api
type Client struct {
	options    clientOptions
	httpClient *http.Client
}

// NewClient creates a default client with the provided options
func NewClient(opts ...ClientOption) (*Client, error) {
	c := &Client{httpClient: http.DefaultClient}
	c.setDefaultOptions()
	for _, opt := range opts {
		opt(&c.options)
	}
	if c.options.namespace == "" {
		return nil, fmt.Errorf("Namespace value must be provided")
	}
	c.httpClient.Transport = &http.Transport{
		Proxy: http.ProxyFromEnvironment,
		DialContext: (&net.Dialer{
			Timeout:   30 * time.Second,
			KeepAlive: 30 * time.Second,
			DualStack: true,
		}).DialContext,
		MaxIdleConns:          100,
		IdleConnTimeout:       90 * time.Second,
		TLSHandshakeTimeout:   10 * time.Second,
		ExpectContinueTimeout: 1 * time.Second,
		TLSClientConfig:       &c.options.tlsConfig,
	}
	return c, nil
}

// Namespace returns the namespace name set on the client
func (c *Client) Namespace() string {
	return c.options.namespace
}

func (c *Client) requestWithReader(ctx context.Context, method, url string, reader io.Reader) (*http.Request, error) {
	req, err := http.NewRequest(method, url, reader)
	if err != nil {
		return nil, fmt.Errorf("Error creating request: %s", err)
	}
	req.Header.Add("Content-Type", "application/json")
	req.Header.Add("Api-Key", c.options.apiKey)
	if ctx != nil {
		req = req.WithContext(ctx)
	}
	return req, nil
}

func (c *Client) requestCompletePath(ctx context.Context, method, path string, data interface{}) (*http.Request, error) {
	buf := new(bytes.Buffer)
	if data != nil {
		if err := json.NewEncoder(buf).Encode(data); err != nil {
			return nil, fmt.Errorf("Error encoding data: %s", err)
		}
	}
	return c.requestWithReader(ctx, method, fmt.Sprintf("%s%s", c.options.url, path), buf)
}

func (c *Client) request(ctx context.Context, method, path string, data interface{}) (*http.Request, error) {
	return c.requestCompletePath(ctx, method, fmt.Sprintf("/v1/%s", path), data)
}

type requester func() (*http.Request, error)

func (c *Client) do(requester requester, data interface{}) error {
	req, err := requester()
	if err != nil {
		return err
	}
	if c.options.debug {
		breq, _ := httputil.DumpRequest(req, true)
		log.Print(string(breq))
	}
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("Error making request %s: %s", req.URL, err)
	}
	if c.options.debug {
		bresp, _ := httputil.DumpResponse(resp, true)
		log.Print(string(bresp))
	}
	bb, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("Error reading body: %s", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 400 {
		omegaErr := &OmegaLoadError{}
		err = json.Unmarshal(bb, omegaErr)
		if omegaErr.Code == 0 {
			return fmt.Errorf("Unexpected status code: %d", resp.StatusCode)
		}
		if len(omegaErr.InvalidEntities) == 0 {
			return &omegaErr.OmegaError
		}
		return omegaErr
	}
	if data != nil && resp.StatusCode != http.StatusNoContent {
		err = json.Unmarshal(bb, data)
	}
	return err
}

func (c *Client) setDefaultOptions() {
	defaultOpts := []ClientOption{
		WithAPIKey(os.Getenv(envAPIKey)),
		WithNamespace(os.Getenv(envNamespace)),
		WithURL(os.Getenv(envURL)),
		WithSystemLogger(log.New(ioutil.Discard, "", log.LstdFlags)),
	}
	for _, opt := range defaultOpts {
		opt(&c.options)
	}
}

// OmegaError represents an error ocurring when calling the Omega API
type OmegaError struct {
	Code    int
	Status  int
	Message string
}

func (e *OmegaError) Error() string {
	return fmt.Sprintf("Error: %s (Code: %d)", e.Message, e.Code)
}

// OmegaLoadError represents a specific error that has problems on some entities
type OmegaLoadError struct {
	OmegaError
	InvalidEntities []InvalidEntity
}

// InvalidEntity contains errors ocurring in the entity of the provided position
type InvalidEntity struct {
	Position int
	Errors   []string
	Error    []string
}

func (e *OmegaLoadError) Error() string {
	ie := make([]string, 0)
	for _, v := range e.InvalidEntities {
		errors := strings.Join(v.Error, " and ")
		if v.Errors != nil && len(v.Errors) > 0 {
			errors = strings.Join(v.Errors, " and ")
		}
		ie = append(ie, fmt.Sprintf("<Item in position %d had the following errors: %s>", v.Position, errors))
	}

	return fmt.Sprintf("Error: %s (Code: %d) Invalid Entities: %s", e.Message, e.Code, strings.Join(ie, ","))
}
