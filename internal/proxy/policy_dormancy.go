package proxy

import (
	"net/http"
)

// dormancyRequest contains the required dormant value from a Coder request.
type dormancyRequest struct {
	Dormant *bool `json:"dormant"`
}

// evaluateDormancy allows waking a workspace while refusing requests that mark it dormant.
func (p *Policy) evaluateDormancy(request *http.Request) Decision {
	body, decision := p.readBody(request)
	if decision.Blocked() {
		return decision
	}
	var payload dormancyRequest
	if err := decodeStrictJSON(body, &payload); err != nil {
		return Decision{Status: http.StatusBadRequest, Code: "invalid_request", Message: "Invalid dormancy request.", Detail: err.Error()}
	}
	if payload.Dormant == nil {
		return Decision{Status: http.StatusBadRequest, Code: "invalid_request", Message: "The dormant field is required."}
	}
	if *payload.Dormant {
		return forbidden("workspace_dormancy_forbidden", "Marking a workspace as dormant is managed externally.")
	}
	return Decision{}
}
