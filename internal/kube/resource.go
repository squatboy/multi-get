package kube

import (
	"context"
	"errors"
	"net/http"
	"path"
	"strings"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/dynamic"
	corev1 "k8s.io/client-go/kubernetes/typed/core/v1"
	"k8s.io/client-go/rest"
)

var ErrTableUnsupported = errors.New("table response is not supported")

// ResourceSpec is the canonical result of discovery resolution.
type ResourceSpec struct {
	Input      string
	Resource   string
	Kind       string
	Group      string
	Version    string
	GVR        schema.GroupVersionResource
	Namespaced bool
	Mapping    *meta.RESTMapping
}

// TableResponse is a decoded meta.k8s.io Table page. Rows include the source
// object because includeObject=Object is requested.
type TableResponse struct {
	Table metav1.Table
}

// ResourceClient is the query boundary used by internal/query.
type ResourceClient struct {
	Dynamic    dynamic.Interface
	Table      *rest.RESTClient
	Namespaces corev1.NamespaceInterface
}

func (c *ResourceClient) List(ctx context.Context, spec ResourceSpec, namespace string, opts metav1.ListOptions) (*unstructured.UnstructuredList, error) {
	return c.Dynamic.Resource(spec.GVR).Namespace(namespace).List(ctx, opts)
}

func (c *ResourceClient) Get(ctx context.Context, spec ResourceSpec, namespace, name string) (*unstructured.Unstructured, error) {
	return c.Dynamic.Resource(spec.GVR).Namespace(namespace).Get(ctx, name, metav1.GetOptions{})
}

func (c *ResourceClient) ListTable(ctx context.Context, spec ResourceSpec, namespace string, opts metav1.ListOptions) (*TableResponse, error) {
	if c.Table == nil {
		return nil, ErrTableUnsupported
	}
	request := c.tableRequest(spec, namespace, "")
	addListParams(request, opts)
	var table metav1.Table
	if err := request.Do(ctx).Into(&table); err != nil {
		if isTableUnsupported(err) {
			return nil, ErrTableUnsupported
		}
		return nil, err
	}
	return &TableResponse{Table: table}, nil
}

func (c *ResourceClient) GetTable(ctx context.Context, spec ResourceSpec, namespace, name string) (*TableResponse, error) {
	if c.Table == nil {
		return nil, ErrTableUnsupported
	}
	request := c.tableRequest(spec, namespace, name)
	var table metav1.Table
	if err := request.Do(ctx).Into(&table); err != nil {
		if isTableUnsupported(err) {
			return nil, ErrTableUnsupported
		}
		return nil, err
	}
	return &TableResponse{Table: table}, nil
}

// NamespaceExists is only used after a namespaced GET returns 404. It avoids
// turning a missing object into a missing namespace while keeping explicit -n
// selection free from a preflight Namespace list.
func (c *ResourceClient) NamespaceExists(ctx context.Context, namespace string) (bool, error) {
	if c.Namespaces == nil {
		return true, nil
	}
	_, err := c.Namespaces.Get(ctx, namespace, metav1.GetOptions{})
	if err == nil {
		return true, nil
	}
	if apierrors.IsNotFound(err) {
		return false, nil
	}
	return false, err
}

func (c *ResourceClient) tableRequest(spec ResourceSpec, namespace, name string) *rest.Request {
	apiPath := "/api/" + spec.Version
	if spec.Group != "" {
		apiPath = path.Join("/apis", spec.Group, spec.Version)
	}
	request := c.Table.Get().AbsPath(apiPath)
	if namespace != "" {
		request.Namespace(namespace)
	}
	request.Resource(spec.Resource)
	if name != "" {
		request.Name(name)
	}
	request.Param("includeObject", string(metav1.IncludeObject))
	return request
}

func addListParams(request *rest.Request, opts metav1.ListOptions) {
	if opts.LabelSelector != "" {
		request.Param("labelSelector", opts.LabelSelector)
	}
	if opts.FieldSelector != "" {
		request.Param("fieldSelector", opts.FieldSelector)
	}
	if opts.Continue != "" {
		request.Param("continue", opts.Continue)
	}
}

func isTableUnsupported(err error) bool {
	var status apierrors.APIStatus
	if errors.As(err, &status) {
		code := status.Status().Code
		return code == http.StatusNotAcceptable || code == http.StatusUnsupportedMediaType
	}
	return strings.Contains(strings.ToLower(err.Error()), "406") || strings.Contains(strings.ToLower(err.Error()), "415")
}
