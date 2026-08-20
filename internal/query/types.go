package query

import (
	"context"
	"errors"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"

	"github.com/squatboy/multi-get/internal/kube"
)

type ResourceSpec = kube.ResourceSpec

// ResourceReader is the only API boundary needed by query execution.
type ResourceReader interface {
	List(context.Context, ResourceSpec, string, metav1.ListOptions) (*unstructured.UnstructuredList, error)
	Get(context.Context, ResourceSpec, string, string) (*unstructured.Unstructured, error)
	ListTable(context.Context, ResourceSpec, string, metav1.ListOptions) (*kube.TableResponse, error)
	GetTable(context.Context, ResourceSpec, string, string) (*kube.TableResponse, error)
}

type NamespaceChecker interface {
	NamespaceExists(context.Context, string) (bool, error)
}

type QueryOptions struct {
	Names          []string
	LabelSelector  string
	FieldSelector  string
	SortBy         string
	UseTable       bool
	IgnoreNotFound bool
}

type QueryError struct {
	Resource  string
	Namespace string
	Name      string
	Operation string
	Err       error
}

func (e QueryError) Error() string {
	message := ""
	if e.Err != nil {
		message = e.Err.Error()
	}
	return message
}

func (e QueryError) Unwrap() error { return e.Err }

type TablePage struct {
	Namespace string
	Table     metav1.Table
}

type QueryResult struct {
	Resource      ResourceSpec
	Items         []unstructured.Unstructured
	Errors        []QueryError
	MissingNames  []string
	TablePages    []TablePage
	TableFallback bool
}

func (r *QueryResult) addError(namespace, name, operation string, err error) {
	if err == nil {
		err = errors.New("unknown Kubernetes API error")
	}
	r.Errors = append(r.Errors, QueryError{
		Resource:  r.Resource.Resource,
		Namespace: namespace,
		Name:      name,
		Operation: operation,
		Err:       err,
	})
}
