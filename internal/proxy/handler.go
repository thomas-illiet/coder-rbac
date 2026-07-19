// Package proxy classifies hosts and enforces Coder workspace operation policies.
package proxy

import (
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httputil"
	"time"

	"github.com/thomas-illiet/coder-rbac/internal/versiongate"
)

// Handler enforces host, version, and user policies before forwarding requests.
type Handler struct {
	classifier *HostClassifier
	policy     *Policy
	gate       *versiongate.Gate
	upstream   *httputil.ReverseProxy
	logger     *slog.Logger
	metrics    *Metrics
}

// NewHandler creates the HTTP handler that protects the upstream reverse proxy.
func NewHandler(classifier *HostClassifier, policy *Policy, gate *versiongate.Gate, upstream *httputil.ReverseProxy, logger *slog.Logger, metrics *Metrics) *Handler {
	return &Handler{classifier: classifier, policy: policy, gate: gate, upstream: upstream, logger: logger, metrics: metrics}
}

// ServeHTTP validates the request and forwards it only when every policy allows it.
func (h *Handler) ServeHTTP(response http.ResponseWriter, request *http.Request) {
	started := time.Now()
	hostKind := "unclassified"
	outcome := "rejected"
	defer func() {
		h.metrics.observeRequest(request.Method, hostKind, outcome, time.Since(started))
	}()

	if ready, _, err := h.gate.Ready(); !ready {
		h.logger.Warn("request rejected while version gate is unready", "method", request.Method, "host", request.Host, "error", err)
		decision := Decision{Status: http.StatusServiceUnavailable, Code: "coder_version_unavailable", Message: "The Coder version policy is not ready."}
		h.reject(response, decision)
		return
	}

	normalizedPath, err := NormalizePath(request)
	if err != nil {
		decision := Decision{Status: http.StatusBadRequest, Code: "invalid_path", Message: "The request path is invalid.", Detail: err.Error()}
		h.reject(response, decision)
		return
	}
	kind := h.classifier.Classify(request.Host)
	hostKind = kind.String()
	switch kind {
	case HostUnknown:
		decision := Decision{Status: http.StatusMisdirectedRequest, Code: "unknown_host", Message: "The requested host is not served by this proxy."}
		h.reject(response, decision)
		return
	case HostWildcard:
		if wildcardControlPath(normalizedPath) {
			h.metrics.observeRejection("wildcard_control_path")
			http.NotFound(response, request)
			return
		}
	}
	if (kind == HostMain || kind == HostInternal) && admissionExemptPath(request.Method, normalizedPath) {
		outcome = "forwarded"
		h.upstream.ServeHTTP(response, request)
		return
	}

	access, decision := h.policy.Authorize(request.Context(), request)
	if decision.Blocked() {
		h.reject(response, decision)
		return
	}
	if kind == HostMain || kind == HostInternal {
		decision = h.policy.Evaluate(request.Context(), request, normalizedPath, access)
		if decision.Blocked() {
			h.reject(response, decision)
			return
		}
	}
	outcome = "forwarded"
	h.upstream.ServeHTTP(response, request)
}

// reject records and writes a structured policy rejection.
func (h *Handler) reject(response http.ResponseWriter, decision Decision) {
	h.metrics.observeRejection(decision.Code)
	writeDecision(response, decision, h.logger)
}

// writeDecision serializes a policy decision as a non-cacheable JSON response.
func writeDecision(response http.ResponseWriter, decision Decision, logger *slog.Logger) {
	if decision.Status >= 500 && decision.Detail != "" {
		logger.Error("policy dependency failed", "code", decision.Code, "error", decision.Detail)
		decision.Detail = ""
	}
	response.Header().Set("Content-Type", "application/json; charset=utf-8")
	response.Header().Set("Cache-Control", "no-store")
	response.WriteHeader(decision.Status)
	payload := struct {
		Message string `json:"message"`
		Detail  string `json:"detail,omitempty"`
		Code    string `json:"code"`
	}{Message: decision.Message, Detail: decision.Detail, Code: decision.Code}
	_ = json.NewEncoder(response).Encode(payload)
}
