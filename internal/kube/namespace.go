package kube

import (
	"context"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	corev1 "k8s.io/client-go/kubernetes/typed/core/v1"
)

// ListNamespaces implements namespace.NamespaceLister and exposes one API
// page at a time so the selector package owns pagination policy.
func (c *Clients) ListNamespaces(ctx context.Context, continueToken string) ([]string, string, error) {
	list, err := c.NamespaceClient.List(ctx, metav1.ListOptions{Continue: continueToken})
	if err != nil {
		return nil, "", err
	}
	names := make([]string, 0, len(list.Items))
	for _, item := range list.Items {
		names = append(names, item.Name)
	}
	return names, list.Continue, nil
}

// NamespaceLister is useful in tests and documents the client boundary.
type NamespaceLister struct {
	Client corev1.NamespaceInterface
}

func (l NamespaceLister) ListNamespaces(ctx context.Context, continueToken string) ([]string, string, error) {
	list, err := l.Client.List(ctx, metav1.ListOptions{Continue: continueToken})
	if err != nil {
		return nil, "", err
	}
	names := make([]string, 0, len(list.Items))
	for _, item := range list.Items {
		names = append(names, item.Name)
	}
	return names, list.Continue, nil
}
