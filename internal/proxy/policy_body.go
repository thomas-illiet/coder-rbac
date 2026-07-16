package proxy

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
)

// readBody reads a bounded body and restores it so the upstream can consume it unchanged.
func (p *Policy) readBody(request *http.Request) ([]byte, Decision) {
	limited := io.LimitReader(request.Body, p.maxRequestBytes+1)
	body, err := io.ReadAll(limited)
	if err != nil {
		return nil, Decision{Status: http.StatusBadRequest, Code: "invalid_request", Message: "Unable to read the request body.", Detail: err.Error()}
	}
	_ = request.Body.Close()
	request.Body = io.NopCloser(bytes.NewReader(body))
	request.ContentLength = int64(len(body))
	if int64(len(body)) > p.maxRequestBytes {
		return nil, Decision{Status: http.StatusRequestEntityTooLarge, Code: "request_too_large", Message: "The request body is too large for policy inspection."}
	}
	return body, Decision{}
}

// decodeStrictJSON decodes exactly one JSON value and rejects trailing values.
func decodeStrictJSON(body []byte, target any) error {
	decoder := json.NewDecoder(bytes.NewReader(body))
	if err := decoder.Decode(target); err != nil {
		return err
	}
	if decoder.Decode(&struct{}{}) != io.EOF {
		return fmt.Errorf("request body must contain one JSON value")
	}
	return nil
}

// rawJSONHasValue reports whether raw JSON contains a non-null, non-empty value.
func rawJSONHasValue(value json.RawMessage) bool {
	trimmed := bytes.TrimSpace(value)
	return len(trimmed) > 0 && !bytes.Equal(trimmed, []byte("null")) && !bytes.Equal(trimmed, []byte(`""`))
}
