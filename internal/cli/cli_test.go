package cli

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"sync"
	"testing"
)

func TestRunQueriesExplicitNamespacesAndEmitsList(t *testing.T) {
	var mu sync.Mutex
	var resourcePaths []string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, request *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch request.URL.Path {
		case "/api":
			_, _ = fmt.Fprint(w, "{\"kind\":\"APIVersions\",\"apiVersion\":\"v1\",\"versions\":[\"v1\"]}")
		case "/apis":
			_, _ = fmt.Fprint(w, "{\"kind\":\"APIGroupList\",\"apiVersion\":\"v1\",\"groups\":[]}")
		case "/api/v1":
			_, _ = fmt.Fprint(w, "{\"kind\":\"APIResourceList\",\"apiVersion\":\"v1\",\"groupVersion\":\"v1\",\"resources\":[{\"name\":\"pods\",\"singularName\":\"pod\",\"namespaced\":true,\"kind\":\"Pod\",\"verbs\":[\"get\",\"list\"],\"shortNames\":[\"po\"]}]}")
		default:
			if strings.HasPrefix(request.URL.Path, "/api/v1/namespaces/") && strings.HasSuffix(request.URL.Path, "/pods") {
				mu.Lock()
				resourcePaths = append(resourcePaths, request.URL.Path)
				mu.Unlock()
				if request.URL.Query().Get("labelSelector") != "app=api" || request.URL.Query().Get("fieldSelector") != "metadata.name=api" {
					t.Errorf("selectors were not forwarded: %s", request.URL.RawQuery)
				}
				name := strings.Split(request.URL.Path, "/")[4]
				_, _ = fmt.Fprintf(w, "{\"apiVersion\":\"v1\",\"kind\":\"PodList\",\"metadata\":{},\"items\":[{\"apiVersion\":\"v1\",\"kind\":\"Pod\",\"metadata\":{\"namespace\":%q,\"name\":\"api\"}}]}", name)
				return
			}
			w.WriteHeader(http.StatusNotFound)
			_, _ = fmt.Fprint(w, "{\"kind\":\"Status\",\"apiVersion\":\"v1\",\"status\":\"Failure\",\"code\":404}")
		}
	}))
	defer server.Close()

	var stdout, stderr strings.Builder
	code := Run(context.Background(), []string{
		"get", "pods", "-n", "dev,stage", "-l", "app=api", "--field-selector", "metadata.name=api", "-o", "json",
		"--server", server.URL, "--cache-dir", t.TempDir(),
	}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("exit=%d stderr=%q", code, stderr.String())
	}
	var list map[string]interface{}
	if err := json.Unmarshal([]byte(stdout.String()), &list); err != nil {
		t.Fatalf("stdout=%q: %v", stdout.String(), err)
	}
	if list["kind"] != "List" {
		t.Fatalf("list %#v", list)
	}
	items := list["items"].([]interface{})
	if len(items) != 2 {
		t.Fatalf("items %#v", items)
	}
	mu.Lock()
	paths := append([]string(nil), resourcePaths...)
	mu.Unlock()
	if want := []string{"/api/v1/namespaces/dev/pods", "/api/v1/namespaces/stage/pods"}; !reflect.DeepEqual(paths, want) {
		t.Fatalf("paths %#v, want %#v", paths, want)
	}
}
