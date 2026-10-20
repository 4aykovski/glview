package httpx

import (
	"context"
	"errors"
	"net/http"
	"testing"
	"time"

	"github.com/4aykovski/glview/internal/gitlab"
)

func TestFromStatus(t *testing.T) {
	cases := []struct {
		status int
		body   string
		kind   error
		msg    string
	}{
		{401, `{"message":"401 Unauthorized"}`, gitlab.ErrUnauthorized, "401 Unauthorized"},
		{403, `{"error":"insufficient_scope","error_description":"The request requires higher privileges than provided by the access token."}`, gitlab.ErrInsufficientScope, "The request requires higher privileges than provided by the access token."},
		{403, `{"message":"403 Forbidden"}`, gitlab.ErrForbidden, "403 Forbidden"},
		{404, `{"message":"404 Project Not Found"}`, gitlab.ErrNotFound, "404 Project Not Found"},
		{409, `{"message":"SHA does not match HEAD of source branch"}`, gitlab.ErrConflict, "SHA does not match HEAD of source branch"},
		{405, `{"message":"405 Method Not Allowed"}`, gitlab.ErrNotPossible, "405 Method Not Allowed"},
		{422, `{"message":"Branch cannot be merged"}`, gitlab.ErrNotPossible, "Branch cannot be merged"},
		{400, `{"message":{"tag_name":["is missing"]}}`, gitlab.ErrNotPossible, "tag_name is missing"},
		{502, `<html>bad gateway</html>`, gitlab.ErrServer, ""},
	}
	for _, c := range cases {
		err := FromStatus(c.status, http.Header{}, []byte(c.body))
		if !errors.Is(err, c.kind) {
			t.Errorf("%d: kind %v, want %v", c.status, err, c.kind)
		}
		var ge *gitlab.Error
		if !errors.As(err, &ge) || ge.Message != c.msg || ge.Status != c.status {
			t.Errorf("%d: %+v, want message %q", c.status, ge, c.msg)
		}
	}
}

func TestInsufficientScopeIsAlsoForbidden(t *testing.T) {
	err := FromStatus(403, nil, []byte(`{"error":"insufficient_scope"}`))
	if !errors.Is(err, gitlab.ErrForbidden) || !errors.Is(err, gitlab.ErrInsufficientScope) {
		t.Fatalf("err = %v", err)
	}
}

func TestRateLimitedCarriesRetryAfter(t *testing.T) {
	h := http.Header{}
	h.Set("Retry-After", "7")
	var ge *gitlab.Error
	if !errors.As(FromStatus(429, h, nil), &ge) || ge.RetryAfter != 7*time.Second {
		t.Fatalf("%+v", ge)
	}
}

func TestFromTransportKeepsCancel(t *testing.T) {
	if err := FromTransport(context.Canceled); !errors.Is(err, context.Canceled) {
		t.Fatal("cancel must pass through")
	}
	if err := FromTransport(errors.New("dial tcp: no such host")); !errors.Is(err, gitlab.ErrNetwork) {
		t.Fatalf("want ErrNetwork, got %v", err)
	}
}
