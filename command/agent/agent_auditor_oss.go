package agent

import (
	"context"
	"fmt"
	"io/fs"
	"net/http"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/hashicorp/eventlogger"
	hclog "github.com/hashicorp/go-hclog"
	"github.com/hashicorp/go-uuid"
	"github.com/hashicorp/nomad/command/agent/event"
	"github.com/hashicorp/nomad/nomad/structs/config"
	"github.com/ryanuber/go-glob"
)

type eventerAuditor struct {
	broker           *eventlogger.Broker
	enabled          bool
	deliveryEnforced bool

	lock   sync.RWMutex
	logger hclog.Logger
}

func newEventerAuditor(cfg *config.AuditConfig, log hclog.Logger) (event.Auditor, error) {
	var enabled bool
	if cfg.Enabled != nil && *cfg.Enabled {
		enabled = true
	}

	e := &eventerAuditor{
		enabled: enabled,
		logger:  log,
	}
	if enabled {
		if err := e.configureBroker(cfg); err != nil {
			return nil, fmt.Errorf("error configuring event broker: %w", err)
		}
	}
	return e, nil
}

func (e *eventerAuditor) configureBroker(cfg *config.AuditConfig) error {
	broker, err := eventlogger.NewBroker()
	if err != nil {
		return err
	}

	pipelineID, err := uuid.GenerateUUID()
	if err != nil {
		return err
	}

	nodes := []eventlogger.NodeID{}

	for _, filter := range cfg.Filters {
		filterNodeID, err := uuid.GenerateUUID()
		if err != nil {
			return err
		}
		filterNode := newFilterNode(filter)
		if err = broker.RegisterNode(eventlogger.NodeID(filterNodeID), filterNode); err != nil {
			return err
		}
		nodes = append(nodes, eventlogger.NodeID(filterNodeID))
	}

	formatterNodeID, err := uuid.GenerateUUID()
	if err != nil {
		return err
	}
	if err = broker.RegisterNode(eventlogger.NodeID(formatterNodeID), &eventlogger.JSONFormatter{}); err != nil {
		return err
	}
	nodes = append(nodes, eventlogger.NodeID(formatterNodeID))

	var deliveryEnforced bool
	for _, sink := range cfg.Sinks {
		sinkNodeID, err := uuid.GenerateUUID()
		if err != nil {
			return err
		}
		sinkNode, err := newSinkNode(sink)
		if err != nil {
			return err
		}
		if err = broker.RegisterNode(eventlogger.NodeID(sinkNodeID), sinkNode); err != nil {
			return err
		}
		nodes = append(nodes, eventlogger.NodeID(sinkNodeID))
		deliveryEnforced = deliveryEnforced || sink.DeliveryGuarantee == "enforced"
	}

	pipeline := eventlogger.Pipeline{
		PipelineID: eventlogger.PipelineID(pipelineID),
		EventType:  "audit",
		NodeIDs:    nodes,
	}
	if err = broker.RegisterPipeline(pipeline); err != nil {
		return err
	}

	e.lock.Lock()
	e.deliveryEnforced = deliveryEnforced
	e.broker = broker
	e.lock.Unlock()
	return nil
}

func (e *eventerAuditor) Event(ctx context.Context, eventType string, payload interface{}) error {
	_, err := e.broker.Send(ctx, eventlogger.EventType(eventType), payload)
	return err
}

func (e *eventerAuditor) Enabled() bool {
	e.lock.RLock()
	defer e.lock.RUnlock()
	return e.enabled
}

func (e *eventerAuditor) SetEnabled(enabled bool) {
	e.lock.Lock()
	e.enabled = enabled
	e.lock.Unlock()
}

func (e *eventerAuditor) Reopen() error {
	return e.broker.Reopen(context.TODO())
}

func (e *eventerAuditor) DeliveryEnforced() bool {
	e.lock.RLock()
	defer e.lock.RUnlock()
	return e.deliveryEnforced
}

func newFilterNode(filterCfg *config.AuditFilter) *eventlogger.Filter {
	return &eventlogger.Filter{
		Predicate: func(e *eventlogger.Event) (bool, error) {
			eventRecv := e.Payload.(*AuditEvent)

			if filterCfg.Type != eventRecv.Type {
				return true, nil
			}
			if !containsKeyOrGlob(eventRecv.Request.Operation, filterCfg.Operations, false) {
				return true, nil
			}
			if !containsKeyOrGlob(eventRecv.Stage, filterCfg.Stages, false) {
				return true, nil
			}
			if !containsKeyOrGlob(eventRecv.Request.Endpoint, filterCfg.Endpoints, true) {
				return true, nil
			}
			return false, nil
		},
	}
}

func containsKeyOrGlob(key string, list []string, extra bool) bool {
	for _, e := range list {
		if e == "*" {
			return true
		}
		if key == e {
			return true
		}
		if extra && strings.HasPrefix(key, e) {
			return true
		}
		if extra && glob.Glob(e, key) {
			return true
		}
	}
	return false
}

func newSinkNode(sinkCfg *config.AuditSink) (*eventlogger.FileSink, error) {
	mode, err := strconv.ParseUint(sinkCfg.Mode, 8, 12)
	if err != nil {
		return nil, fmt.Errorf("error parsing file mode: %w", err)
	}

	return &eventlogger.FileSink{
		Path:        filepath.Dir(sinkCfg.Path),
		FileName:    filepath.Base(sinkCfg.Path),
		Mode:        fs.FileMode(mode),
		MaxBytes:    sinkCfg.RotateBytes,
		MaxFiles:    sinkCfg.RotateMaxFiles,
		MaxDuration: sinkCfg.RotateDuration,
	}, nil
}

type AuditEvent struct {
	ID        string              `json:"id"`
	Type      string              `json:"type"`
	Stage     string              `json:"stage"`
	Timestamp time.Time           `json:"timestamp"`
	Version   int                 `json:"version"`
	Auth      *AuthAuditEvent     `json:"auth,omitempty"`
	Request   *RequestAuditEvent  `json:"request"`
	Response  *ResponseAuditEvent `json:"response,omitempty"`
}

type AuthAuditEvent struct {
	AccessorID string    `json:"accessor_id"`
	Name       string    `json:"name"`
	Global     bool      `json:"global"`
	Policies   []string  `json:"policies,omitempty"`
	CreateTime time.Time `json:"create_time"`
	// TODO: roles?
}

type RequestAuditEvent struct {
	ID          string                 `json:"id"`
	Operation   string                 `json:"operation"`
	Endpoint    string                 `json:"endpoint"`
	Namespace   NamespaceAuditEvent    `json:"namespace"`
	RequestMeta map[string]interface{} `json:"request_meta,omitempty"`
	NodeMeta    map[string]interface{} `json:"node_meta,omitempty"`
}

type ResponseAuditEvent struct {
	StatusCode int    `json:"status_code"`
	Error      string `json:"error,omitempty"`
}

type NamespaceAuditEvent struct {
	ID string `json:"id"`
}

func (s *HTTPServer) newAuditEventFromRequest(req *http.Request) (*AuditEvent, error) {
	auditEventID, err := uuid.GenerateUUID()
	if err != nil {
		return nil, err
	}

	var namespace string
	parseNamespace(req, &namespace)

	e := &AuditEvent{
		ID:        auditEventID,
		Type:      "HTTPEvent",
		Stage:     "OperationReceived",
		Timestamp: time.Now(),
		Version:   1,
		Request: &RequestAuditEvent{
			Operation: req.Method,
			Endpoint:  req.URL.Path,
			Namespace: NamespaceAuditEvent{
				ID: namespace,
			},
			RequestMeta: map[string]interface{}{
				"remote_address": req.RemoteAddr,
				"user_agent":     req.UserAgent(),
			},
			NodeMeta: map[string]interface{}{"ip": s.listener.Addr().String()},
		},
	}

	// TODO: req.Context().Value(ContextKeyReqID)
	reqID, err := uuid.GenerateUUID()
	if err != nil {
		return nil, err
	}
	e.Request.ID = reqID

	var secret string
	s.parseToken(req, &secret)

	resolver := s.agent.Client().ResolveSecretToken
	if server := s.agent.Server(); server != nil {
		resolver = server.ResolveSecretToken
	}

	token, err := resolver(secret)
	if err != nil {
		// TODO: this returns an error if the token does not exist
		// return nil, err
	}
	if token != nil {
		// TODO: set also claims for when this is not a token
		e.Auth = &AuthAuditEvent{
			AccessorID: token.AccessorID,
			Name:       token.Name,
			Global:     token.Global,
			Policies:   token.Policies,
			CreateTime: token.CreateTime,
		}
	}

	return e, nil
}

func (e *AuditEvent) AddResponse(statusCode int, statusErr error) (*AuditEvent, error) {
	auditEventID, err := uuid.GenerateUUID()
	if err != nil {
		return nil, err
	}
	e.ID = auditEventID
	e.Stage = "OperationComplete"
	e.Response = &ResponseAuditEvent{
		StatusCode: statusCode,
	}
	if statusErr != nil {
		e.Response.Error = statusErr.Error()
	}
	return e, nil
}
