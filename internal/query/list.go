package query

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"

	"github.com/squatboy/multi-get/internal/kube"
)

// Run executes one canonical resource across namespaces. Namespace and name
// loops are deliberately sequential so output order follows the request.
func Run(ctx context.Context, reader ResourceReader, spec ResourceSpec, namespaces []string, opts QueryOptions) QueryResult {
	result := QueryResult{Resource: spec}
	if len(opts.Names) > 0 {
		runGet(ctx, reader, &result, namespaces, opts)
	} else {
		runList(ctx, reader, &result, namespaces, opts)
	}
	if opts.SortBy != "" && len(opts.Names) == 0 {
		if err := SortItems(result.Items, opts.SortBy); err != nil {
			result.addError("", "", "Sort", err)
		}
	}
	return result
}

func runList(ctx context.Context, reader ResourceReader, result *QueryResult, namespaces []string, opts QueryOptions) {
	tableEnabled := opts.UseTable
	for _, namespace := range namespaces {
		continueToken := ""
		for {
			listOpts := metav1.ListOptions{
				LabelSelector: opts.LabelSelector,
				FieldSelector: opts.FieldSelector,
				Continue:      continueToken,
			}
			if tableEnabled {
				tableResponse, err := reader.ListTable(ctx, result.Resource, namespace, listOpts)
				if err == nil {
					items, itemErr := itemsFromTable(tableResponse.Table, namespace)
					if itemErr == nil {
						result.Items = append(result.Items, items...)
						result.TablePages = append(result.TablePages, TablePage{Namespace: namespace, Table: tableResponse.Table})
						continueToken = tableResponse.Table.Continue
						if continueToken == "" {
							break
						}
						continue
					}
					// A Table without includeObject cannot safely feed a List
					// aggregate, so use the generic response for this and later pages.
					tableEnabled = false
					result.TableFallback = true
				} else if errors.Is(err, kube.ErrTableUnsupported) {
					tableEnabled = false
					result.TableFallback = true
				} else {
					result.addError(namespace, "", "List", err)
					break
				}
			}

			list, err := reader.List(ctx, result.Resource, namespace, listOpts)
			if err != nil {
				result.addError(namespace, "", "List", err)
				break
			}
			appendItems(result, namespace, list.Items)
			continueToken = list.GetContinue()
			if continueToken == "" {
				break
			}
		}
	}
}

func runGet(ctx context.Context, reader ResourceReader, result *QueryResult, namespaces []string, opts QueryOptions) {
	found := make(map[string]bool, len(opts.Names))
	uncertain := make(map[string]bool, len(opts.Names))
	tableEnabled := opts.UseTable
	for _, namespace := range namespaces {
		for _, name := range opts.Names {
			var item *unstructured.Unstructured
			var err error
			if tableEnabled {
				var tableResponse *kube.TableResponse
				tableResponse, err = reader.GetTable(ctx, result.Resource, namespace, name)
				if err == nil {
					items, itemErr := itemsFromTable(tableResponse.Table, namespace)
					if itemErr == nil && len(items) > 0 {
						result.Items = append(result.Items, items...)
						item = &result.Items[len(result.Items)-1]
						result.TablePages = append(result.TablePages, TablePage{Namespace: namespace, Table: tableResponse.Table})
					} else {
						tableEnabled = false
						result.TableFallback = true
					}
				} else if errors.Is(err, kube.ErrTableUnsupported) {
					tableEnabled = false
					result.TableFallback = true
				}
			}
			if item == nil && (err == nil || errors.Is(err, kube.ErrTableUnsupported) || result.TableFallback) {
				item, err = reader.Get(ctx, result.Resource, namespace, name)
			}
			if err != nil {
				if apierrors.IsNotFound(err) && !isMissingNamespace(ctx, reader, namespace, err) {
					continue
				}
				uncertain[name] = true
				result.addError(namespace, name, "Get", err)
				continue
			}
			if item == nil {
				continue
			}
			item.SetNamespace(namespaceIfEmpty(item.GetNamespace(), namespace))
			found[name] = true
			// Table rows are already appended above. Generic fallback needs the
			// object exactly once as well.
			if !containsItem(result.Items, item) {
				result.Items = append(result.Items, *item)
			}
		}
	}

	for _, name := range opts.Names {
		if found[name] || uncertain[name] {
			continue
		}
		result.MissingNames = append(result.MissingNames, name)
		if !opts.IgnoreNotFound {
			result.addError("", name, "Get", fmt.Errorf("%q was not found in the selected namespaces", name))
		}
	}
}

func appendItems(result *QueryResult, namespace string, items []unstructured.Unstructured) {
	for _, item := range items {
		item.SetNamespace(namespaceIfEmpty(item.GetNamespace(), namespace))
		result.Items = append(result.Items, item)
	}
}

func itemsFromTable(table metav1.Table, namespace string) ([]unstructured.Unstructured, error) {
	items := make([]unstructured.Unstructured, 0, len(table.Rows))
	for _, row := range table.Rows {
		raw := row.Object.Raw
		if len(raw) == 0 && row.Object.Object != nil {
			var err error
			raw, err = json.Marshal(row.Object.Object)
			if err != nil {
				return nil, err
			}
		}
		if len(raw) == 0 {
			return nil, errors.New("Table row did not include its object")
		}
		var object map[string]interface{}
		if err := json.Unmarshal(raw, &object); err != nil {
			return nil, err
		}
		item := unstructured.Unstructured{Object: object}
		item.SetNamespace(namespaceIfEmpty(item.GetNamespace(), namespace))
		items = append(items, item)
	}
	return items, nil
}

func isMissingNamespace(ctx context.Context, reader ResourceReader, namespace string, err error) bool {
	if status, ok := err.(apierrors.APIStatus); ok {
		details := status.Status().Details
		if details != nil && (strings.EqualFold(details.Kind, "namespace") || strings.EqualFold(details.Kind, "namespaces")) {
			return details.Name == namespace
		}
	}
	checker, ok := reader.(NamespaceChecker)
	if !ok {
		return false
	}
	exists, checkErr := checker.NamespaceExists(ctx, namespace)
	return checkErr == nil && !exists
}

func namespaceIfEmpty(value, fallback string) string {
	if value == "" {
		return fallback
	}
	return value
}

func containsItem(items []unstructured.Unstructured, wanted *unstructured.Unstructured) bool {
	for _, item := range items {
		if item.GetNamespace() == wanted.GetNamespace() && item.GetName() == wanted.GetName() {
			return true
		}
	}
	return false
}
