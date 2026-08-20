package query

import (
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
)

func Aggregate(result QueryResult) *unstructured.UnstructuredList {
	list := &unstructured.UnstructuredList{}
	list.SetAPIVersion("v1")
	list.SetKind("List")
	list.Items = append([]unstructured.Unstructured(nil), result.Items...)
	return list
}
