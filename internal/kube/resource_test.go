package kube

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/rest"
)

func TestListTableRequestsTableRepresentationAndDecodesRows(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, request *http.Request) {
		if request.URL.Path != "/api/v1/namespaces/dev/pods" {
			t.Errorf("path %q", request.URL.Path)
		}
		if !strings.Contains(request.Header.Get("Accept"), "as=Table") {
			t.Errorf("Accept header %q", request.Header.Get("Accept"))
		}
		if request.URL.Query().Get("includeObject") != "Object" {
			t.Errorf("query %q", request.URL.RawQuery)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = fmt.Fprint(w, `{"apiVersion":"meta.k8s.io/v1","kind":"Table","columnDefinitions":[{"name":"NAME","type":"string","priority":0}],"rows":[{"cells":["api"],"object":{"apiVersion":"v1","kind":"Pod","metadata":{"namespace":"dev","name":"api"}}}]}`)
	}))
	defer server.Close()

	tableClient, err := newTableClient(&rest.Config{Host: server.URL})
	if err != nil {
		t.Fatal(err)
	}
	client := &ResourceClient{Table: tableClient}
	response, err := client.ListTable(context.Background(), ResourceSpec{
		Group: "", Version: "v1", Resource: "pods", GVR: schema.GroupVersionResource{Version: "v1", Resource: "pods"}, Namespaced: true,
	}, "dev", metav1.ListOptions{LabelSelector: "app=api", FieldSelector: "metadata.name=api"})
	if err != nil {
		t.Fatal(err)
	}
	if len(response.Table.Rows) != 1 || response.Table.Rows[0].Object.Raw == nil {
		t.Fatalf("table response %#v", response.Table)
	}
}

func TestListTableReturnsUnsupportedSentinel(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNotAcceptable)
		_, _ = fmt.Fprint(w, `{"kind":"Status","apiVersion":"v1","status":"Failure","code":406}`)
	}))
	defer server.Close()
	tableClient, err := newTableClient(&rest.Config{Host: server.URL})
	if err != nil {
		t.Fatal(err)
	}
	client := &ResourceClient{Table: tableClient}
	_, err = client.ListTable(context.Background(), ResourceSpec{Version: "v1", Resource: "pods"}, "dev", metav1.ListOptions{})
	if err != ErrTableUnsupported {
		t.Fatalf("error %v, want ErrTableUnsupported", err)
	}
}
