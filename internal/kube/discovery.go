package kube

import (
	"fmt"
	"sort"
	"strings"

	"k8s.io/apimachinery/pkg/api/meta"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/discovery"
)

type discoveredResource struct {
	resource   string
	kind       string
	group      string
	version    string
	namespaced bool
	shortNames []string
}

// Resolver applies the resource-name precedence from the CLI contract before
// asking the REST mapper for the final API mapping.
type Resolver struct {
	Discovery discovery.DiscoveryInterface
	Mapper    meta.RESTMapper
}

func (r Resolver) Resolve(input string) (ResourceSpec, error) {
	if strings.TrimSpace(input) == "" {
		return ResourceSpec{}, fmt.Errorf("resource must not be empty")
	}
	if strings.Contains(input, "/") {
		return ResourceSpec{}, fmt.Errorf("subresource %q is not supported", input)
	}

	resources, err := r.preferredResources()
	if err != nil {
		return ResourceSpec{}, err
	}
	qualifiedResource, qualifiedGroup := splitQualifiedName(input)
	candidates := make([]discoveredResource, 0)
	bestPriority := 100
	for _, candidate := range resources {
		priority, ok := matchPriority(candidate, qualifiedResource, qualifiedGroup, strings.Contains(input, "."))
		if !ok {
			continue
		}
		if priority < bestPriority {
			bestPriority = priority
			candidates = candidates[:0]
		}
		if priority == bestPriority {
			candidates = append(candidates, candidate)
		}
	}
	if len(candidates) == 0 {
		return ResourceSpec{}, fmt.Errorf("unknown resource %q; available candidates: %s", input, candidateNames(resources))
	}
	if len(candidates) > 1 {
		return ResourceSpec{}, fmt.Errorf("resource %q is ambiguous; candidates: %s", input, candidateNames(candidates))
	}

	candidate := candidates[0]
	gvr := schema.GroupVersionResource{Group: candidate.group, Version: candidate.version, Resource: candidate.resource}
	var mapping *meta.RESTMapping
	if r.Mapper != nil {
		mapped, mapErr := r.Mapper.RESTMapping(schema.GroupKind{Group: candidate.group, Kind: candidate.kind}, candidate.version)
		if mapErr != nil {
			return ResourceSpec{}, fmt.Errorf("map resource %q: %w", input, mapErr)
		}
		mapping = mapped
		gvr = mapped.Resource
	}
	namespaced := candidate.namespaced
	if mapping != nil {
		namespaced = mapping.Scope.Name() == meta.RESTScopeNameNamespace
	}
	if !namespaced {
		return ResourceSpec{}, fmt.Errorf("resource %q is cluster-scoped; only namespaced resources are supported", input)
	}

	return ResourceSpec{
		Input:      input,
		Resource:   candidate.resource,
		Kind:       candidate.kind,
		Group:      candidate.group,
		Version:    candidate.version,
		GVR:        gvr,
		Namespaced: namespaced,
		Mapping:    mapping,
	}, nil
}

func (r Resolver) preferredResources() ([]discoveredResource, error) {
	if r.Discovery == nil {
		return nil, fmt.Errorf("discovery client is not configured")
	}
	lists, err := r.Discovery.ServerPreferredResources()
	if err != nil && len(lists) == 0 {
		return nil, fmt.Errorf("discover API resources: %w", err)
	}
	resources := make([]discoveredResource, 0)
	for _, list := range lists {
		gv, parseErr := schema.ParseGroupVersion(list.GroupVersion)
		if parseErr != nil {
			continue
		}
		for _, resource := range list.APIResources {
			if strings.Contains(resource.Name, "/") {
				continue
			}
			resources = append(resources, discoveredResource{
				resource:   resource.Name,
				kind:       resource.Kind,
				group:      gv.Group,
				version:    gv.Version,
				namespaced: resource.Namespaced,
				shortNames: append([]string(nil), resource.ShortNames...),
			})
		}
	}
	if len(resources) == 0 && err != nil {
		return nil, fmt.Errorf("discover API resources: %w", err)
	}
	return resources, nil
}

func splitQualifiedName(input string) (string, string) {
	parts := strings.SplitN(input, ".", 2)
	if len(parts) == 1 {
		return input, ""
	}
	return parts[0], parts[1]
}

func matchPriority(candidate discoveredResource, input, group string, qualified bool) (int, bool) {
	if qualified {
		if !strings.EqualFold(candidate.group, group) {
			return 0, false
		}
		if strings.EqualFold(candidate.resource, input) {
			return 0, true
		}
		for _, short := range candidate.shortNames {
			if strings.EqualFold(short, input) {
				return 1, true
			}
		}
		if strings.EqualFold(candidate.kind, input) {
			return 2, true
		}
		return 0, false
	}
	if candidate.resource == input {
		if candidate.group == "" {
			return 0, true
		}
		// Core resources are the unqualified kubectl default when an API
		// group exposes the same plural resource (for example pods and
		// pods.metrics.k8s.io).
		return 1, true
	}
	for _, short := range candidate.shortNames {
		if short == input {
			return 1, true
		}
	}
	if strings.EqualFold(candidate.kind, input) {
		return 2, true
	}
	return 0, false
}

func candidateNames(resources []discoveredResource) string {
	names := make([]string, 0, len(resources))
	seen := make(map[string]struct{}, len(resources))
	for _, resource := range resources {
		name := resource.resource
		if resource.group != "" {
			name += "." + resource.group
		}
		if _, ok := seen[name]; ok {
			continue
		}
		seen[name] = struct{}{}
		names = append(names, name)
	}
	sort.Strings(names)
	return strings.Join(names, ", ")
}
