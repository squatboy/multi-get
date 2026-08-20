package namespace

import (
	"context"
	"reflect"
	"testing"
)

type fakeLister struct {
	pages map[string]struct {
		names []string
		next  string
	}
	calls []string
}

func (f *fakeLister) ListNamespaces(_ context.Context, token string) ([]string, string, error) {
	f.calls = append(f.calls, token)
	page := f.pages[token]
	return page.names, page.next, nil
}

func TestResolveExplicitTrimsAndDeduplicatesWithoutListing(t *testing.T) {
	lister := &fakeLister{}
	got, err := Resolve(context.Background(), Options{Explicit: "a, b,a", ExplicitSet: true}, lister)
	if err != nil {
		t.Fatal(err)
	}
	if want := []string{"a", "b"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("got %#v, want %#v", got, want)
	}
	if len(lister.calls) != 0 {
		t.Fatalf("explicit selection listed namespaces: %#v", lister.calls)
	}
}

func TestResolveFindUsesSubstringAndSorts(t *testing.T) {
	lister := &fakeLister{pages: map[string]struct {
		names []string
		next  string
	}{"": {names: []string{"dev-b", "dev-a", "prod-dev", "default"}}}}
	got, err := Resolve(context.Background(), Options{FindNS: "dev", FindNSSet: true}, lister)
	if err != nil {
		t.Fatal(err)
	}
	if want := []string{"dev-a", "dev-b", "prod-dev"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("got %#v, want %#v", got, want)
	}

	got, err = Resolve(context.Background(), Options{FindNS: "prod", FindNSSet: true}, lister)
	if err != nil {
		t.Fatal(err)
	}
	if want := []string{"prod-dev"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("got %#v, want %#v", got, want)
	}
}

func TestResolveFindRejectsEmptySearchText(t *testing.T) {
	lister := &fakeLister{}
	_, err := Resolve(context.Background(), Options{FindNSSet: true}, lister)
	if err == nil || err.Error() != "namespace search text must not be empty" {
		t.Fatalf("err=%v, want empty search text error", err)
	}
	if len(lister.calls) != 0 {
		t.Fatalf("empty search text listed namespaces: %#v", lister.calls)
	}
}

func TestResolvePaginationAndAllSort(t *testing.T) {
	lister := &fakeLister{pages: map[string]struct {
		names []string
		next  string
	}{
		"":     {names: []string{"z"}, next: "next"},
		"next": {names: []string{"a"}},
	}}
	got, err := Resolve(context.Background(), Options{All: true}, lister)
	if err != nil {
		t.Fatal(err)
	}
	if want := []string{"a", "z"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("got %#v, want %#v", got, want)
	}
	if want := []string{"", "next"}; !reflect.DeepEqual(lister.calls, want) {
		t.Fatalf("calls %#v, want %#v", lister.calls, want)
	}
}
