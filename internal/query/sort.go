package query

import (
	"fmt"
	"reflect"
	"sort"
	"strings"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/client-go/util/jsonpath"
)

func ValidateSortBy(expression string) error {
	_, err := newJSONPath(expression)
	return err
}

func SortItems(items []unstructured.Unstructured, expression string) error {
	path, err := newJSONPath(expression)
	if err != nil {
		return err
	}
	type keyed struct {
		item    unstructured.Unstructured
		value   string
		missing bool
	}
	keyedItems := make([]keyed, 0, len(items))
	for _, item := range items {
		value, missing, valueErr := jsonPathValue(path, item.Object)
		if valueErr != nil {
			return valueErr
		}
		keyedItems = append(keyedItems, keyed{item: item, value: value, missing: missing})
	}
	sort.SliceStable(keyedItems, func(i, j int) bool {
		left, right := keyedItems[i], keyedItems[j]
		if left.missing != right.missing {
			return !left.missing
		}
		if left.value != right.value {
			return left.value < right.value
		}
		if left.item.GetNamespace() != right.item.GetNamespace() {
			return left.item.GetNamespace() < right.item.GetNamespace()
		}
		return left.item.GetName() < right.item.GetName()
	})
	for i := range keyedItems {
		items[i] = keyedItems[i].item
	}
	return nil
}

func newJSONPath(expression string) (*jsonpath.JSONPath, error) {
	expression = strings.TrimSpace(expression)
	if expression == "" {
		return nil, fmt.Errorf("sort expression must not be empty")
	}
	if !strings.HasPrefix(expression, "{") {
		expression = "{" + expression + "}"
	}
	path := jsonpath.New("sort").AllowMissingKeys(true)
	return path, path.Parse(expression)
}

func jsonPathValue(path *jsonpath.JSONPath, object map[string]interface{}) (string, bool, error) {
	values, err := path.FindResults(object)
	if err != nil {
		return "", false, err
	}
	if len(values) == 0 || len(values[0]) == 0 {
		return "", true, nil
	}
	value := values[0][0]
	if !value.IsValid() {
		return "", true, nil
	}
	for value.Kind() == reflect.Interface || value.Kind() == reflect.Pointer {
		if value.IsNil() {
			return "", true, nil
		}
		value = value.Elem()
	}
	return fmt.Sprint(value.Interface()), false, nil
}
