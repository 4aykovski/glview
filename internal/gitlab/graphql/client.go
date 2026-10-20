// Package graphql is a thin GitLab GraphQL client: one POST per query,
// error decoding, and a batcher that queries many projects at once with one
// alias per project, adapting the batch size to GitLab's complexity limit.
package graphql

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/4aykovski/glview/internal/debuglog"
	"github.com/4aykovski/glview/internal/gitlab"
	"github.com/4aykovski/glview/internal/gitlab/httpx"
)

// Client talks to {baseURL}/api/graphql.
type Client struct {
	endpoint string
	token    string
	hc       *http.Client

	// DefaultBatch is the starting number of projects per batch request.
	DefaultBatch int
	// Parallel is the number of batch requests in flight at once.
	Parallel int
	// RequestTimeout bounds a single GraphQL request (GitLab's own limit is 30s).
	RequestTimeout time.Duration

	mu      sync.Mutex
	learned map[string]int  // batch size learned per query kind
	missing map[string]bool // optional fields the instance does not know
}

// New creates a client sharing the http client (and its limits) with REST.
func New(baseURL, token string, hc *http.Client, batchSize int) *Client {
	if batchSize <= 0 {
		batchSize = 10
	}
	return &Client{
		endpoint:       strings.TrimRight(baseURL, "/") + "/api/graphql",
		token:          token,
		hc:             hc,
		DefaultBatch:   batchSize,
		Parallel:       3,
		RequestTimeout: 30 * time.Second,
		learned:        map[string]int{},
		missing:        map[string]bool{},
	}
}

// Error is one entry of the GraphQL "errors" array.
type Error struct {
	Message string `json:"message"`
	Path    []any  `json:"path"`
}

// Response is a raw GraphQL response.
type Response struct {
	Data   json.RawMessage `json:"data"`
	Errors []Error         `json:"errors"`
}

// ComplexityError reports a query rejected for exceeding the complexity limit.
type ComplexityError struct {
	Score, Limit int
}

func (e *ComplexityError) Error() string {
	return fmt.Sprintf("query complexity %d exceeds limit %d", e.Score, e.Limit)
}

func (e *ComplexityError) Is(target error) bool { return target == gitlab.ErrComplexity }

var complexityRe = regexp.MustCompile(`complexity of (\d+), which exceeds max complexity of (\d+)`)

// Raw executes a query and returns data and errors as they came. Transport
// and HTTP errors are returned as *gitlab.Error; a complexity rejection as
// *ComplexityError. Other GraphQL errors are left in Response.Errors.
func (c *Client) Raw(ctx context.Context, query string, vars map[string]any) (*Response, error) {
	body, err := json.Marshal(map[string]any{"query": query, "variables": vars})
	if err != nil {
		return nil, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.endpoint, bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")
	req.Header.Set("Authorization", "Bearer "+c.token)

	resp, err := c.hc.Do(req)
	if err != nil {
		return nil, httpx.FromTransport(err)
	}
	defer resp.Body.Close()
	data, err := io.ReadAll(io.LimitReader(resp.Body, 32<<20))
	if err != nil {
		return nil, httpx.FromTransport(err)
	}
	if resp.StatusCode != http.StatusOK {
		return nil, httpx.FromStatus(resp.StatusCode, resp.Header, data)
	}

	var out Response
	if err := json.Unmarshal(data, &out); err != nil {
		return nil, fmt.Errorf("decode GraphQL response: %w", err)
	}
	for _, e := range out.Errors {
		debuglog.L().Debug("graphql error", "op", operationName(query), "message", e.Message, "path", e.Path)
		if m := complexityRe.FindStringSubmatch(e.Message); m != nil {
			score, _ := strconv.Atoi(m[1])
			limit, _ := strconv.Atoi(m[2])
			return nil, &ComplexityError{Score: score, Limit: limit}
		}
	}
	return &out, nil
}

// operationName returns the name after "query" for logs ("ProjectMRs").
func operationName(query string) string {
	q := strings.TrimSpace(query)
	q = strings.TrimPrefix(q, "query")
	q = strings.TrimSpace(q)
	if i := strings.IndexAny(q, "( {"); i >= 0 {
		q = q[:i]
	}
	return q
}

// Do executes a query and decodes data into out. Any GraphQL error fails the call.
func (c *Client) Do(ctx context.Context, query string, vars map[string]any, out any) error {
	resp, err := c.Raw(ctx, query, vars)
	if err != nil {
		return err
	}
	return decode(resp, out)
}

func decodeData(resp *Response, out any) error {
	if out == nil || len(resp.Data) == 0 || string(resp.Data) == "null" {
		return nil
	}
	if err := json.Unmarshal(resp.Data, out); err != nil {
		return fmt.Errorf("decode GraphQL data: %w", err)
	}
	return nil
}

func (c *Client) batchSize(kind string) int {
	c.mu.Lock()
	defer c.mu.Unlock()
	if n, ok := c.learned[kind]; ok {
		return n
	}
	return c.DefaultBatch
}

func (c *Client) learn(kind string, n int) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if cur, ok := c.learned[kind]; !ok || n < cur {
		c.learned[kind] = n
	}
}

// errIsTimeout reports a per-request deadline (not a parent cancellation).
func errIsTimeout(err error) bool {
	if errors.Is(err, context.DeadlineExceeded) {
		return true
	}
	var ge *gitlab.Error
	if errors.As(err, &ge) && ge.Kind == gitlab.ErrNetwork {
		var ne interface{ Timeout() bool }
		return errors.As(ge.Err, &ne) && ne.Timeout()
	}
	return false
}
