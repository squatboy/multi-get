package kube

import (
	"testing"

	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/discovery"
	"k8s.io/client-go/discovery/fake"
)

type preferredDiscovery struct {
	discovery.DiscoveryInterface
	resources []*metav1.APIResourceList
}

func (d preferredDiscovery) ServerPreferredResources() ([]*metav1.APIResourceList, error) {
	return d.resources, nil
}

func TestResolverUsesResourceShortNameAndKindPrecedence(t *testing.T) {
	resources := []*metav1.APIResourceList{{
		GroupVersion: "v1",
		APIResources: []metav1.APIResource{{Name: "pods", Kind: "Pod", Namespaced: true, ShortNames: []string{"po"}}},
	}, {
		GroupVersion: "apps/v1",
		APIResources: []metav1.APIResource{{Name: "deployments", Kind: "Deployment", Namespaced: true, ShortNames: []string{"deploy"}}},
	}}
	mapper := meta.NewDefaultRESTMapper([]schema.GroupVersion{{Version: "v1"}, {Group: "apps", Version: "v1"}})
	mapper.Add(schema.GroupVersionKind{Version: "v1", Kind: "Pod"}, meta.RESTScopeNamespace)
	mapper.Add(schema.GroupVersionKind{Group: "apps", Version: "v1", Kind: "Deployment"}, meta.RESTScopeNamespace)
	resolver := Resolver{Discovery: preferredDiscovery{DiscoveryInterface: &fake.FakeDiscovery{}, resources: resources}, Mapper: mapper}

	spec, err := resolver.Resolve("po")
	if err != nil || spec.Resource != "pods" || spec.Kind != "Pod" {
		t.Fatalf("spec %#v, err %v", spec, err)
	}
	spec, err = resolver.Resolve("deployment.apps")
	if err != nil || spec.GVR.Group != "apps" || spec.Resource != "deployments" {
		t.Fatalf("spec %#v, err %v", spec, err)
	}
	spec, err = resolver.Resolve("deployment")
	if err != nil || spec.Resource != "deployments" {
		t.Fatalf("spec %#v, err %v", spec, err)
	}
}

func TestResolverRejectsSubresourceAndClusterScope(t *testing.T) {
	resources := []*metav1.APIResourceList{{
		GroupVersion: "v1",
		APIResources: []metav1.APIResource{
			{Name: "nodes", Kind: "Node", Namespaced: false},
			{Name: "pods", Kind: "Pod", Namespaced: true},
		},
	}}
	mapper := meta.NewDefaultRESTMapper([]schema.GroupVersion{{Version: "v1"}})
	mapper.Add(schema.GroupVersionKind{Version: "v1", Kind: "Node"}, meta.RESTScopeRoot)
	mapper.Add(schema.GroupVersionKind{Version: "v1", Kind: "Pod"}, meta.RESTScopeNamespace)
	resolver := Resolver{Discovery: preferredDiscovery{DiscoveryInterface: &fake.FakeDiscovery{}, resources: resources}, Mapper: mapper}
	if _, err := resolver.Resolve("pods/status"); err == nil {
		t.Fatal("expected subresource error")
	}
	if _, err := resolver.Resolve("nodes"); err == nil {
		t.Fatal("expected cluster-scope error")
	}
}
