package query

import (
	"context"
	"reflect"
	"testing"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"

	"github.com/squatboy/multi-get/internal/kube"
)

type fakeReader struct {
	pages      map[string][]unstructured.UnstructuredList
	listErrors map[string]error
	getObjects map[string]*unstructured.Unstructured
	getErrors  map[string]error
	listCalls  []string
	getCalls   []string
}

func (f *fakeReader) List(_ context.Context, _ ResourceSpec, namespace string, opts metav1.ListOptions) (*unstructured.UnstructuredList, error) {
	key := namespace + ":" + opts.Continue
	f.listCalls = append(f.listCalls, key)
	if err := f.listErrors[namespace]; err != nil {
		return nil, err
	}
	pages := f.pages[namespace]
	index := 0
	if opts.Continue != "" {
		index = 1
	}
	if index >= len(pages) {
		return &unstructured.UnstructuredList{}, nil
	}
	return pages[index].DeepCopy(), nil
}

func (f *fakeReader) Get(_ context.Context, _ ResourceSpec, namespace, name string) (*unstructured.Unstructured, error) {
	key := namespace + "/" + name
	f.getCalls = append(f.getCalls, key)
	if err := f.getErrors[key]; err != nil {
		return nil, err
	}
	if object := f.getObjects[key]; object != nil {
		return object.DeepCopy(), nil
	}
	return nil, apierrors.NewNotFound(schema.GroupResource{Group: "", Resource: "pods"}, name)
}

func (f *fakeReader) ListTable(context.Context, ResourceSpec, string, metav1.ListOptions) (*kube.TableResponse, error) {
	return nil, kube.ErrTableUnsupported
}

func (f *fakeReader) GetTable(context.Context, ResourceSpec, string, string) (*kube.TableResponse, error) {
	return nil, kube.ErrTableUnsupported
}

func TestRunListPreservesNamespaceAndPageOrder(t *testing.T) {
	reader := &fakeReader{pages: map[string][]unstructured.UnstructuredList{
		"dev": {
			list([]unstructured.Unstructured{item("dev", "a")}, "next"),
			list([]unstructured.Unstructured{item("dev", "b")}, ""),
		},
		"stage": {list([]unstructured.Unstructured{item("stage", "c")}, "")},
	}}
	result := Run(context.Background(), reader, testSpec(), []string{"dev", "stage"}, QueryOptions{LabelSelector: "app=api", FieldSelector: "metadata.name=a"})
	if len(result.Errors) != 0 {
		t.Fatalf("unexpected errors: %#v", result.Errors)
	}
	if got := itemKeys(result.Items); !reflect.DeepEqual(got, []string{"dev/a", "dev/b", "stage/c"}) {
		t.Fatalf("items %#v", got)
	}
	if want := []string{"dev:", "dev:next", "stage:"}; !reflect.DeepEqual(reader.listCalls, want) {
		t.Fatalf("list calls %#v, want %#v", reader.listCalls, want)
	}
}

func TestRunListCollectsPartialErrors(t *testing.T) {
	reader := &fakeReader{
		pages: map[string][]unstructured.UnstructuredList{
			"dev": {list([]unstructured.Unstructured{item("dev", "a")}, "")},
		},
		listErrors: map[string]error{"stage": apierrors.NewForbidden(schema.GroupResource{Resource: "pods"}, "", nil)},
	}
	result := Run(context.Background(), reader, testSpec(), []string{"dev", "stage"}, QueryOptions{})
	if got := itemKeys(result.Items); !reflect.DeepEqual(got, []string{"dev/a"}) {
		t.Fatalf("items %#v", got)
	}
	if len(result.Errors) != 1 || result.Errors[0].Namespace != "stage" || result.Errors[0].Operation != "List" {
		t.Fatalf("errors %#v", result.Errors)
	}
}

func TestRunGetNamespaceThenNameOrderAndIgnoreNotFound(t *testing.T) {
	reader := &fakeReader{
		getObjects: map[string]*unstructured.Unstructured{
			"dev/api":   pointer(item("dev", "api")),
			"stage/web": pointer(item("stage", "web")),
		},
	}
	result := Run(context.Background(), reader, testSpec(), []string{"dev", "stage"}, QueryOptions{
		Names:          []string{"api", "web"},
		IgnoreNotFound: true,
	})
	if len(result.Errors) != 0 {
		t.Fatalf("unexpected errors: %#v", result.Errors)
	}
	if got := itemKeys(result.Items); !reflect.DeepEqual(got, []string{"dev/api", "stage/web"}) {
		t.Fatalf("items %#v", got)
	}
	if want := []string{"dev/api", "dev/web", "stage/api", "stage/web"}; !reflect.DeepEqual(reader.getCalls, want) {
		t.Fatalf("get calls %#v, want %#v", reader.getCalls, want)
	}
	if len(result.MissingNames) != 0 {
		t.Fatalf("missing names %#v", result.MissingNames)
	}
}

func TestSortItemsMissingValuesLast(t *testing.T) {
	items := []unstructured.Unstructured{
		itemWithValue("stage", "z", "status", "Running"),
		item("dev", "a"),
		itemWithValue("dev", "b", "status", "Pending"),
	}
	if err := SortItems(items, ".status"); err != nil {
		t.Fatal(err)
	}
	if got := itemKeys(items); !reflect.DeepEqual(got, []string{"dev/b", "stage/z", "dev/a"}) {
		t.Fatalf("items %#v", got)
	}
}

func testSpec() ResourceSpec {
	return ResourceSpec{
		Resource:   "pods",
		Kind:       "Pod",
		Version:    "v1",
		GVR:        schema.GroupVersionResource{Version: "v1", Resource: "pods"},
		Namespaced: true,
	}
}

func list(items []unstructured.Unstructured, continueToken string) unstructured.UnstructuredList {
	result := unstructured.UnstructuredList{Items: items}
	result.SetContinue(continueToken)
	return result
}

func item(namespace, name string) unstructured.Unstructured {
	return unstructured.Unstructured{Object: map[string]interface{}{
		"apiVersion": "v1",
		"kind":       "Pod",
		"metadata": map[string]interface{}{
			"namespace": namespace,
			"name":      name,
		},
	}}
}

func itemWithValue(namespace, name, key, value string) unstructured.Unstructured {
	result := item(namespace, name)
	result.Object[key] = value
	return result
}

func pointer(value unstructured.Unstructured) *unstructured.Unstructured { return &value }

func itemKeys(items []unstructured.Unstructured) []string {
	result := make([]string, 0, len(items))
	for _, item := range items {
		result = append(result, item.GetNamespace()+"/"+item.GetName())
	}
	return result
}
