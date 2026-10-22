package graphql

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strings"
	"sync"

	"github.com/4aykovski/glview/internal/gitlab"
)

// BatchSpec describes a per-project query run over many projects.
type BatchSpec[T any] struct {
	// Kind identifies the query for remembering the learned batch size
	// ("ws.mrs", "ws.mrs.mine", "ws.pipelines", ...).
	Kind string
	// Fragment is a full fragment definition on Project, e.g.
	// "fragment WsMRGroup on Project { ... }".
	Fragment string
	// FragmentName is the name used in the spread (…WsMRGroup).
	FragmentName string
	// VarDecls declares shared variables used by the fragment,
	// e.g. "$first: Int!, $me: String!". May be empty.
	VarDecls string
	// Vars holds the values for VarDecls.
	Vars map[string]any
	// PerProject optionally declares variables of one alias: i is the
	// alias index in the request, path the project. It returns the
	// declarations ("$r3: String!") and their values ({"r3": "main"}).
	// Fragments take no arguments, so per-alias variables are used in Body.
	PerProject func(i int, path string) (decl string, vars map[string]any)
	// Body optionally returns the selection of alias i; by default it is
	// the fragment spread ("...FragmentName"). A body that does not use
	// the fragment requires an empty Fragment.
	Body func(i int) string
	// Decode turns one project's JSON object into items and a total count.
	Decode func(path string, raw json.RawMessage) (items []T, total int, err error)
}

// BuildQuery returns the batch query and variables for paths.
func BuildQuery[T any](spec BatchSpec[T], paths []string) (string, map[string]any) {
	var b strings.Builder
	decls := make([]string, 0, len(paths)+1)
	if spec.VarDecls != "" {
		decls = append(decls, spec.VarDecls)
	}
	vars := make(map[string]any, len(spec.Vars)+len(paths))
	for k, v := range spec.Vars {
		vars[k] = v
	}
	for i, p := range paths {
		decls = append(decls, fmt.Sprintf("$p%d: ID!", i))
		vars[fmt.Sprintf("p%d", i)] = p
		if spec.PerProject != nil {
			decl, pv := spec.PerProject(i, p)
			if decl != "" {
				decls = append(decls, decl)
			}
			for k, v := range pv {
				vars[k] = v
			}
		}
	}
	fmt.Fprintf(&b, "query Batch(%s) {\n", strings.Join(decls, ", "))
	for i := range paths {
		body := "..." + spec.FragmentName
		if spec.Body != nil {
			body = spec.Body(i)
		}
		fmt.Fprintf(&b, "p%d: project(fullPath: $p%d) { %s }\n", i, i, body)
	}
	b.WriteString("}\n")
	b.WriteString(spec.Fragment)
	return b.String(), vars
}

// Batch runs spec over paths. Results arrive on the returned channel batch
// by batch, in no particular order between batches; the channel is closed
// when all batches finished or ctx was cancelled.
func Batch[T any](ctx context.Context, c *Client, spec BatchSpec[T], paths []string) <-chan gitlab.BatchResult[T] {
	out := make(chan gitlab.BatchResult[T])
	go func() {
		defer close(out)
		size := c.batchSize(spec.Kind)
		chunks := split(paths, size)

		sem := make(chan struct{}, max(1, c.Parallel))
		var wg sync.WaitGroup
		for _, chunk := range chunks {
			select {
			case sem <- struct{}{}:
			case <-ctx.Done():
				wg.Wait()
				return
			}
			wg.Add(1)
			go func() {
				defer wg.Done()
				defer func() { <-sem }()
				runChunk(ctx, c, spec, chunk, true, out)
			}()
		}
		wg.Wait()
	}()
	return out
}

// runChunk executes one batch request, shrinking and retrying on complexity
// errors and splitting once on timeout.
func runChunk[T any](ctx context.Context, c *Client, spec BatchSpec[T], paths []string, mayRetryTimeout bool, out chan<- gitlab.BatchResult[T]) {
	query, vars := BuildQuery(spec, paths)
	text, names := c.render(query)
	reqCtx, cancel := context.WithTimeout(ctx, c.RequestTimeout)
	resp, err := c.Raw(reqCtx, text, vars)
	cancel()

	// An optional field unknown to the instance: drop it and repeat.
	if err == nil && len(resp.Errors) > 0 && c.dropUnknown(resp.Errors, names) != "" {
		runChunk(ctx, c, spec, paths, mayRetryTimeout, out)
		return
	}

	var ce *ComplexityError
	if errors.As(err, &ce) && len(paths) > 1 {
		n := shrink(len(paths), ce.Score, ce.Limit)
		c.learn(spec.Kind, n)
		for _, sub := range split(paths, n) {
			runChunk(ctx, c, spec, sub, mayRetryTimeout, out)
		}
		return
	}
	if err != nil && ctx.Err() == nil && errIsTimeout(err) && mayRetryTimeout && len(paths) > 1 {
		half := (len(paths) + 1) / 2
		runChunk(ctx, c, spec, paths[:half], false, out)
		runChunk(ctx, c, spec, paths[half:], false, out)
		return
	}

	var res gitlab.BatchResult[T]
	if err != nil {
		res.Err = err
		res.Groups = make([]gitlab.ProjectGroup[T], len(paths))
		for i, p := range paths {
			res.Groups[i] = gitlab.ProjectGroup[T]{Project: gitlab.ProjectRef{FullPath: p}, Total: -1, Err: err}
		}
	} else {
		res = decodeBatch(spec, paths, resp)
	}
	select {
	case out <- res:
	case <-ctx.Done():
	}
}

func decodeBatch[T any](spec BatchSpec[T], paths []string, resp *Response) gitlab.BatchResult[T] {
	var data map[string]json.RawMessage
	_ = json.Unmarshal(resp.Data, &data)

	// Errors addressed to a specific alias belong to that project only.
	aliasErr := map[string][]string{}
	var global []string
	for _, e := range resp.Errors {
		if len(e.Path) > 0 {
			if alias, ok := e.Path[0].(string); ok && strings.HasPrefix(alias, "p") {
				aliasErr[alias] = append(aliasErr[alias], e.Message)
				continue
			}
		}
		global = append(global, e.Message)
	}

	res := gitlab.BatchResult[T]{Groups: make([]gitlab.ProjectGroup[T], len(paths))}
	if len(global) > 0 && len(data) == 0 {
		res.Err = &gitlab.Error{Kind: gitlab.ErrServer, Message: strings.Join(global, "; ")}
	}
	for i, p := range paths {
		alias := fmt.Sprintf("p%d", i)
		g := gitlab.ProjectGroup[T]{Project: gitlab.ProjectRef{FullPath: p}, Total: -1}
		raw := data[alias]
		switch {
		case res.Err != nil:
			g.Err = res.Err
		case len(raw) == 0 || string(raw) == "null":
			if msgs := aliasErr[alias]; len(msgs) > 0 {
				g.Err = &gitlab.Error{Kind: gitlab.ErrServer, Message: strings.Join(msgs, "; ")}
			} else {
				g.Err = &gitlab.Error{Kind: gitlab.ErrNotFound}
			}
		default:
			items, total, err := spec.Decode(p, raw)
			g.Items, g.Total, g.Err = items, total, err
			if g.Err == nil && len(aliasErr[alias]) > 0 {
				g.Err = &gitlab.Error{Kind: gitlab.ErrServer, Message: strings.Join(aliasErr[alias], "; ")}
			}
		}
		res.Groups[i] = g
	}
	return res
}

// shrink returns the next batch size after a complexity rejection.
func shrink(n, score, limit int) int {
	if score <= 0 || limit <= 0 {
		return max(1, n/2)
	}
	next := int(float64(n) * float64(limit) / float64(score) * 0.9)
	if next >= n {
		next = n - 1
	}
	return max(1, next)
}

func split(paths []string, size int) [][]string {
	if size <= 0 {
		size = 1
	}
	var out [][]string
	for start := 0; start < len(paths); start += size {
		out = append(out, paths[start:min(start+size, len(paths))])
	}
	return out
}

// SortGroups orders groups by the given project order (stable UI order
// independent of which batch arrived first).
func SortGroups[T any](groups []gitlab.ProjectGroup[T], order []string) {
	idx := make(map[string]int, len(order))
	for i, p := range order {
		idx[p] = i
	}
	sort.SliceStable(groups, func(i, j int) bool {
		return idx[groups[i].Project.FullPath] < idx[groups[j].Project.FullPath]
	})
}
