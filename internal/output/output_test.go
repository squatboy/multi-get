package output

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"

	"github.com/squatboy/multi-get/internal/query"
)

func TestRenderGenericTableAlwaysIncludesNamespace(t *testing.T) {
	result := testResult([]unstructured.Unstructured{testItem("dev", "api")})
	result.TableFallback = true
	var output bytes.Buffer
	if err := Render(&output, []query.QueryResult{result}, Options{}); err != nil {
		t.Fatal(err)
	}
	text := output.String()
	if !strings.Contains(text, "NAMESPACE") || !strings.Contains(text, "dev") {
		t.Fatalf("output %q", text)
	}
}

func TestRenderStructuredListAndName(t *testing.T) {
	result := testResult([]unstructured.Unstructured{testItem("stage", "api")})
	var jsonOutput bytes.Buffer
	if err := Render(&jsonOutput, []query.QueryResult{result}, Options{Format: "json"}); err != nil {
		t.Fatal(err)
	}
	var decoded map[string]interface{}
	if err := json.Unmarshal(jsonOutput.Bytes(), &decoded); err != nil {
		t.Fatal(err)
	}
	if decoded["kind"] != "List" {
		t.Fatalf("decoded %#v", decoded)
	}
	items, ok := decoded["items"].([]interface{})
	if !ok || len(items) != 1 || items[0].(map[string]interface{})["metadata"].(map[string]interface{})["namespace"] != "stage" {
		t.Fatalf("decoded %#v", decoded)
	}

	var nameOutput bytes.Buffer
	if err := Render(&nameOutput, []query.QueryResult{result}, Options{Format: "name"}); err != nil {
		t.Fatal(err)
	}
	if got := nameOutput.String(); got != "pod/stage/api\n" {
		t.Fatalf("name output %q", got)
	}
}

func TestRenderMultiResourceUsesBlocks(t *testing.T) {
	first := testResult([]unstructured.Unstructured{testItem("dev", "api")})
	second := testResult([]unstructured.Unstructured{testItem("dev", "api")})
	second.Resource.Resource = "services"
	second.Resource.Kind = "Service"
	var output bytes.Buffer
	if err := Render(&output, []query.QueryResult{first, second}, Options{Multi: true}); err != nil {
		t.Fatal(err)
	}
	text := output.String()
	if !strings.Contains(text, "pods:") || !strings.Contains(text, "services:") {
		t.Fatalf("output %q", text)
	}
}

func TestValidateRejectsStructuredMultiResource(t *testing.T) {
	if err := Validate(Options{Format: "json", Multi: true}); err == nil {
		t.Fatal("expected validation error")
	}
}

func testResult(items []unstructured.Unstructured) query.QueryResult {
	return query.QueryResult{
		Resource: query.ResourceSpec{
			Resource: "pods",
			Kind:     "Pod",
			GVR:      schema.GroupVersionResource{Version: "v1", Resource: "pods"},
		},
		Items: items,
	}
}

func testItem(namespace, name string) unstructured.Unstructured {
	return unstructured.Unstructured{Object: map[string]interface{}{
		"apiVersion": "v1",
		"kind":       "Pod",
		"metadata": map[string]interface{}{
			"namespace": namespace,
			"name":      name,
		},
	}}
}
