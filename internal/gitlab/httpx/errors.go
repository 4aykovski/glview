package httpx

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"sort"
	"strings"
	"time"

	"github.com/4aykovski/glview/internal/gitlab"
)

// FromStatus maps an HTTP error response to a *gitlab.Error.
func FromStatus(status int, header http.Header, body []byte) error {
	e := &gitlab.Error{Status: status, Message: message(body)}
	switch status {
	case http.StatusUnauthorized:
		e.Kind = gitlab.ErrUnauthorized
	case http.StatusForbidden:
		e.Kind = gitlab.ErrForbidden
		if strings.Contains(string(body), "insufficient_scope") {
			e.Kind = gitlab.ErrInsufficientScope
		}
	case http.StatusNotFound:
		e.Kind = gitlab.ErrNotFound
	case http.StatusConflict:
		e.Kind = gitlab.ErrConflict
	case http.StatusMethodNotAllowed, http.StatusUnprocessableEntity, http.StatusNotAcceptable, http.StatusBadRequest:
		e.Kind = gitlab.ErrNotPossible
	case http.StatusTooManyRequests:
		e.Kind = gitlab.ErrRateLimited
		if d, ok := RetryAfter(header, time.Now()); ok {
			e.RetryAfter = d
		}
	default:
		e.Kind = gitlab.ErrServer
	}
	return e
}

// FromTransport maps a transport-level error (DNS, TLS, timeout) to a
// *gitlab.Error. Context cancellation is returned unchanged so callers can
// tell a user abort from a failure.
func FromTransport(err error) error {
	if err == nil {
		return nil
	}
	if errors.Is(err, context.Canceled) {
		return err
	}
	var certErr *tls.CertificateVerificationError
	var unknownAuth x509.UnknownAuthorityError
	var hostErr x509.HostnameError
	var invalid x509.CertificateInvalidError
	if errors.As(err, &certErr) || errors.As(err, &unknownAuth) || errors.As(err, &hostErr) || errors.As(err, &invalid) {
		return &gitlab.Error{Kind: gitlab.ErrTLS, Err: err}
	}
	return &gitlab.Error{Kind: gitlab.ErrNetwork, Err: err}
}

// message extracts GitLab's error text from a JSON body:
// {"message": "..."}, {"message": {"field": ["..."]}}, {"error": "..."}.
func message(body []byte) string {
	var v struct {
		Message any    `json:"message"`
		Error   string `json:"error"`
		Desc    string `json:"error_description"`
	}
	if err := json.Unmarshal(body, &v); err != nil {
		s := strings.TrimSpace(string(body))
		if len(s) > 200 || strings.HasPrefix(s, "<") {
			return ""
		}
		return s
	}
	switch m := v.Message.(type) {
	case string:
		return m
	case map[string]any:
		keys := make([]string, 0, len(m))
		for k := range m {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		var parts []string
		for _, k := range keys {
			parts = append(parts, fmt.Sprintf("%s %v", k, flatten(m[k])))
		}
		return strings.Join(parts, "; ")
	}
	if v.Desc != "" {
		return v.Desc
	}
	return v.Error
}

func flatten(v any) string {
	if list, ok := v.([]any); ok {
		parts := make([]string, len(list))
		for i, x := range list {
			parts[i] = fmt.Sprint(x)
		}
		return strings.Join(parts, ", ")
	}
	return fmt.Sprint(v)
}
