// Copyright (c) HashiCorp, Inc.
// SPDX-License-Identifier: BUSL-1.1

//go:build !ent
// +build !ent

package agent

import (
	"context"
	"net/http"
	"strings"

	"github.com/felixge/httpsnoop"
	"github.com/hashicorp/nomad/nomad/structs"
)

const (
	// ErrUnableToAudit is used when the event was not able to be
	// audited and it is enforced
	ErrUnableToAudit = "Unable to audit event"
)

// registerEnterpriseHandlers is a no-op for the oss release
func (s *HTTPServer) registerEnterpriseHandlers() {
	s.mux.HandleFunc("/v1/sentinel/policies", s.wrap(s.SentinelPoliciesRequest))
	s.mux.HandleFunc("/v1/sentinel/policy/", s.wrap(s.SentinelPolicySpecificRequest))

	s.mux.HandleFunc("/v1/quotas", s.wrap(s.entOnly))
	s.mux.HandleFunc("/v1/quota-usages", s.wrap(s.entOnly))
	s.mux.HandleFunc("/v1/quota/", s.wrap(s.entOnly))
	s.mux.HandleFunc("/v1/quota", s.wrap(s.entOnly))

	s.mux.HandleFunc("/v1/recommendation", s.wrap(s.entOnly))
	s.mux.HandleFunc("/v1/recommendations", s.wrap(s.entOnly))
	s.mux.HandleFunc("/v1/recommendations/apply", s.wrap(s.entOnly))
	s.mux.HandleFunc("/v1/recommendation/", s.wrap(s.entOnly))
}

func (s *HTTPServer) entOnly(resp http.ResponseWriter, req *http.Request) (interface{}, error) {
	return nil, CodedError(501, ErrEntOnly)
}

func (s *HTTPServer) auditRequest(req *http.Request) (*AuditEvent, error) {
	event, err := s.newAuditEventFromRequest(req)
	if err != nil {
		s.logger.Error("failed to create audit event from request", "error", err)
		if s.eventAuditor.DeliveryEnforced() {
			return nil, err
		}
	}
	if event != nil {
		if err := s.eventAuditor.Event(req.Context(), "audit", event); err != nil {
			s.logger.Error("failed event in event auditor", "error", err)
			if s.eventAuditor.DeliveryEnforced() {
				return nil, err
			}
		}
	}
	return event, nil
}

func codeFromErr(statusCode int, err error) int {
	if err == nil {
		return statusCode
	}

	code := 500
	errMsg := err.Error()
	if http, ok := err.(HTTPCodedError); ok {
		code = http.Code()
	} else if ecode, emsg, ok := structs.CodeFromRPCCodedErr(err); ok {
		code = ecode
		errMsg = emsg
	} else {
		// RPC errors get wrapped, so manually unwrap by only looking at their suffix
		if strings.HasSuffix(errMsg, structs.ErrPermissionDenied.Error()) {
			code = 403
		} else if strings.HasSuffix(errMsg, structs.ErrTokenNotFound.Error()) {
			code = 403
		} else if strings.HasSuffix(errMsg, structs.ErrJobRegistrationDisabled.Error()) {
			code = 403
		} else if strings.HasSuffix(errMsg, structs.ErrIncompatibleFiltering.Error()) {
			code = 400
		}
	}
	return code
}

func (s *HTTPServer) auditResponse(ctx context.Context, event *AuditEvent, statusCode int, statusErr error) error {
	if event != nil {
		event, err := event.AddResponse(codeFromErr(statusCode, statusErr), statusErr)
		if err != nil {
			s.logger.Error("failed to create audit event from response", "error", err)
			if s.eventAuditor.DeliveryEnforced() {
				return err
			}
		}
		if event != nil {
			if err := s.eventAuditor.Event(ctx, "audit", event); err != nil {
				s.logger.Error("failed event in event auditor", "error", err)
				if s.eventAuditor.DeliveryEnforced() {
					return err
				}
			}
		}
	}
	return nil
}

type respWriterCode struct {
	code int
}

func (rw *respWriterCode) WriteHeader(fw httpsnoop.WriteHeaderFunc) httpsnoop.WriteHeaderFunc {
	return func(code int) {
		rw.code = code
		fw(code)
	}
}

func wrapResponseWriter(resp http.ResponseWriter) (http.ResponseWriter, *respWriterCode) {
	rwc := &respWriterCode{
		code: http.StatusOK,
	}
	hooks := httpsnoop.Hooks{
		WriteHeader: rwc.WriteHeader,
	}
	return httpsnoop.Wrap(resp, hooks), rwc
}

func (s *HTTPServer) unableToAudit(resp http.ResponseWriter, req *http.Request) (interface{}, error) {
	return nil, CodedError(http.StatusInternalServerError, ErrUnableToAudit)
}

// auditHandler wraps the passed handlerFn
func (s *HTTPServer) auditHandler(h handlerFn) handlerFn {
	if !s.eventAuditor.Enabled() {
		return h
	}

	auditFn := func(resp http.ResponseWriter, req *http.Request) (interface{}, error) {
		event, err := s.auditRequest(req)
		if err != nil {
			return s.unableToAudit(resp, req)
		}

		resp, rwc := wrapResponseWriter(resp)
		r, e := h(resp, req)
		if err := s.auditResponse(req.Context(), event, rwc.code, e); err != nil {
			return s.unableToAudit(resp, req)
		}
		return r, e
	}

	return auditFn
}

func (s *HTTPServer) unableToAuditNonJSON(resp http.ResponseWriter, req *http.Request) ([]byte, error) {
	return nil, CodedError(http.StatusInternalServerError, ErrUnableToAudit)
}

// auditHTTPHandler wraps  the passed handlerByteFn
func (s *HTTPServer) auditNonJSONHandler(h handlerByteFn) handlerByteFn {
	if !s.eventAuditor.Enabled() {
		return h
	}

	auditFn := func(resp http.ResponseWriter, req *http.Request) ([]byte, error) {
		event, err := s.auditRequest(req)
		if err != nil {
			return s.unableToAuditNonJSON(resp, req)
		}

		resp, rwc := wrapResponseWriter(resp)
		r, e := h(resp, req)
		if err := s.auditResponse(req.Context(), event, rwc.code, e); err != nil {
			return s.unableToAuditNonJSON(resp, req)
		}
		return r, e
	}

	return auditFn
}

func (s *HTTPServer) unableToAuditHTTP() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	})
}

// auditHTTPHandler wraps the passed http.Handler
func (s *HTTPServer) auditHTTPHandler(h http.Handler) http.Handler {
	if !s.eventAuditor.Enabled() {
		return h
	}

	auditFn := http.HandlerFunc(func(resp http.ResponseWriter, req *http.Request) {
		event, err := s.auditRequest(req)
		if err != nil {
			s.unableToAuditHTTP()
		}

		resp, rwc := wrapResponseWriter(resp)
		h.ServeHTTP(resp, req)
		if err := s.auditResponse(req.Context(), event, rwc.code, nil); err != nil {
			s.unableToAuditHTTP()
		}
	})

	return auditFn
}
