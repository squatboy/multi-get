package namespace

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"strings"
)

// Options describes the four mutually exclusive namespace selection modes.
type Options struct {
	Explicit string
	FindNS   string
	All      bool
	Current  string

	// The Set fields let callers distinguish an explicitly supplied empty flag
	// from an omitted flag. Existing callers that only set the string fields
	// keep the natural, non-empty behavior.
	ExplicitSet bool
	FindNSSet   bool
}

// NamespaceLister is intentionally small so selector behavior can be tested
// without constructing a Kubernetes client.
type NamespaceLister interface {
	ListNamespaces(ctx context.Context, continueToken string) ([]string, string, error)
}

var ErrNoNamespacesMatched = errors.New("no namespaces matched")

// Resolve returns namespaces in the order required by the selected mode.
func Resolve(ctx context.Context, opts Options, lister NamespaceLister) ([]string, error) {
	explicitSet := opts.ExplicitSet || opts.Explicit != ""
	findSet := opts.FindNSSet || opts.FindNS != ""
	modeCount := 0
	if explicitSet {
		modeCount++
	}
	if findSet {
		modeCount++
	}
	if opts.All {
		modeCount++
	}
	if modeCount > 1 {
		return nil, errors.New("namespace selectors -n/--namespace, --find-ns, and -A are mutually exclusive")
	}

	switch {
	case explicitSet:
		return parseExplicit(opts.Explicit)
	case findSet:
		return findNamespaces(ctx, opts.FindNS, lister)
	case opts.All:
		return listAll(ctx, lister)
	default:
		current := strings.TrimSpace(opts.Current)
		if current == "" {
			current = "default"
		}
		return []string{current}, nil
	}
}

func parseExplicit(value string) ([]string, error) {
	parts := strings.Split(value, ",")
	result := make([]string, 0, len(parts))
	seen := make(map[string]struct{}, len(parts))
	for _, part := range parts {
		name := strings.TrimSpace(part)
		if name == "" {
			return nil, errors.New("namespace list contains an empty name")
		}
		if _, ok := seen[name]; ok {
			continue
		}
		seen[name] = struct{}{}
		result = append(result, name)
	}
	if len(result) == 0 {
		return nil, errors.New("namespace list must not be empty")
	}
	return result, nil
}

func findNamespaces(ctx context.Context, query string, lister NamespaceLister) ([]string, error) {
	query = strings.TrimSpace(query)
	if query == "" {
		return nil, errors.New("namespace search text must not be empty")
	}
	names, err := listAll(ctx, lister)
	if err != nil {
		return nil, err
	}
	matched := names[:0]
	for _, name := range names {
		if strings.Contains(name, query) {
			matched = append(matched, name)
		}
	}
	if len(matched) == 0 {
		return nil, ErrNoNamespacesMatched
	}
	return matched, nil
}

func listAll(ctx context.Context, lister NamespaceLister) ([]string, error) {
	if lister == nil {
		return nil, errors.New("namespace list client is not configured")
	}
	var names []string
	var continueToken string
	for {
		page, next, err := lister.ListNamespaces(ctx, continueToken)
		if err != nil {
			return nil, fmt.Errorf("list namespaces: %w", err)
		}
		names = append(names, page...)
		if next == "" {
			break
		}
		continueToken = next
	}
	sort.Strings(names)
	return names, nil
}
