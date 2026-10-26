package graphql

import (
	"context"
	"errors"
	"fmt"
	"regexp"
	"slices"
	"strconv"
	"strings"

	"github.com/4aykovski/glview/internal/debuglog"
	"github.com/4aykovski/glview/internal/gitlab"
)

// Optional fields. Self-hosted instances differ in schema, so fields that
// may be missing (diffStatsSummary…) are wrapped in marker lines inside the
// query text:
//
//	#@opt diffStatsSummary
//	diffStatsSummary { additions deletions fileCount }
//	#@end
//
// A marker names the field(s) the segment depends on (comma separated).
// When the instance answers "Field 'x' doesn't exist", the segment naming x
// is dropped, the decision is remembered for the session and the request
// is repeated without it. A segment that also names "complexity" is dropped
// the same way when the instance rejects the query as too complex.
const (
	optBegin = "#@opt "
	optEnd   = "#@end"
)

var unknownFieldRe = regexp.MustCompile(`Field '([A-Za-z0-9_]+)' doesn't exist on type`)

// complexityName is the pseudo field name of segments that may be dropped
// when the instance rejects the query as too complex:
//
//	#@opt approvalState,complexity
const complexityName = "complexity"

// DoOpt is Do for queries with optional segments.
func (c *Client) DoOpt(ctx context.Context, query string, vars map[string]any, out any) error {
	for range 8 {
		text, names := c.render(query)
		resp, err := c.Raw(ctx, text, vars)
		if err != nil {
			if c.dropComplex(err, names) {
				continue
			}
			return err
		}
		if len(resp.Errors) > 0 {
			if field := c.dropUnknown(resp.Errors, names); field != "" {
				continue
			}
		}
		return decode(resp, out)
	}
	return &gitlab.Error{Kind: gitlab.ErrServer, Message: "too many optional field retries"}
}

// render strips the markers and drops segments known to be unsupported.
// names lists the optional field names present in the rendered query.
func (c *Client) render(query string) (text string, names []string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	var b strings.Builder
	skip := false
	for line := range strings.SplitSeq(query, "\n") {
		trim := strings.TrimSpace(line)
		switch {
		case strings.HasPrefix(trim, optBegin):
			segNames := splitNames(strings.TrimPrefix(trim, optBegin))
			skip = false
			for _, n := range segNames {
				if c.missing[n] {
					skip = true
				}
			}
			if !skip {
				names = append(names, segNames...)
			}
		case trim == optEnd:
			skip = false
		case !skip:
			b.WriteString(line)
			b.WriteByte('\n')
		}
	}
	return b.String(), names
}

// dropUnknown marks the optional segment owning the unknown field as
// missing. It returns the field name, or "" when the error is not about an
// optional field.
func (c *Client) dropUnknown(errs []Error, names []string) string {
	for _, e := range errs {
		m := unknownFieldRe.FindStringSubmatch(e.Message)
		if m == nil {
			continue
		}
		field := m[1]
		for _, n := range names {
			if n == field {
				c.mu.Lock()
				c.missing[n] = true
				c.mu.Unlock()
				debuglog.L().Info("graphql: optional field missing, retrying without it", "field", field)
				return field
			}
		}
	}
	return ""
}

// dropComplex handles a complexity rejection: when the query has segments
// that may be dropped for it, they are marked missing for the session.
func (c *Client) dropComplex(err error, names []string) bool {
	var ce *ComplexityError
	if !errors.As(err, &ce) || !slices.Contains(names, complexityName) {
		return false
	}
	c.mu.Lock()
	c.missing[complexityName] = true
	c.mu.Unlock()
	debuglog.L().Info("graphql: query too complex, retrying without the optional segments", "score", ce.Score, "limit", ce.Limit)
	return true
}

func splitNames(s string) []string {
	var out []string
	for n := range strings.SplitSeq(s, ",") {
		if n = strings.TrimSpace(n); n != "" {
			out = append(out, n)
		}
	}
	return out
}

// decode turns a response into out, failing on any GraphQL error.
func decode(resp *Response, out any) error {
	if len(resp.Errors) > 0 {
		msgs := make([]string, len(resp.Errors))
		for i, e := range resp.Errors {
			msgs[i] = e.Message
		}
		return &gitlab.Error{Kind: gitlab.ErrServer, Message: strings.Join(msgs, "; ")}
	}
	return decodeData(resp, out)
}

// parseGID extracts the numeric ID from a global ID such as
// "gid://gitlab/Ci::Pipeline/123".
func parseGID(gid string) (int64, error) {
	i := strings.LastIndexByte(gid, '/')
	if i < 0 {
		return 0, fmt.Errorf("invalid global ID %q", gid)
	}
	n, err := strconv.ParseInt(gid[i+1:], 10, 64)
	if err != nil {
		return 0, fmt.Errorf("invalid global ID %q: %w", gid, err)
	}
	return n, nil
}

// gidOrZero is parseGID for fields where a bad ID is not worth failing on.
func gidOrZero(gid string) int64 {
	n, err := parseGID(gid)
	if err != nil {
		return 0
	}
	return n
}

// lower normalizes a GraphQL enum value ("NOT_APPROVED") to the REST form.
func lower(s string) string { return strings.ToLower(s) }
