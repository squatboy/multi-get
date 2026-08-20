package output

import (
	"encoding/json"
	"fmt"
	"io"
	"strings"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/cli-runtime/pkg/printers"

	"github.com/squatboy/multi-get/internal/query"
)

func RenderStructured(w io.Writer, result query.QueryResult, format, expression string) error {
	list := query.Aggregate(result)
	switch format {
	case "json":
		return writeJSON(w, list)
	case "yaml":
		printer := &printers.YAMLPrinter{}
		return printer.PrintObj(list, w)
	case "custom-columns":
		return renderCustomColumns(w, list.Items, expression)
	case "jsonpath":
		printer, err := printers.NewJSONPathPrinter(withTemplateBraces(expression))
		if err != nil {
			return err
		}
		return printer.PrintObj(list, w)
	case "go-template":
		printer, err := printers.NewGoTemplatePrinter([]byte(expression))
		if err != nil {
			return err
		}
		printer.AllowMissingKeys(true)
		return printer.PrintObj(list, w)
	default:
		return fmt.Errorf("unsupported structured output format %q", format)
	}
}

func writeJSON(w io.Writer, list *unstructured.UnstructuredList) error {
	data, err := json.MarshalIndent(list, "", "  ")
	if err != nil {
		return err
	}
	_, err = fmt.Fprintf(w, "%s\n", data)
	return err
}

type customColumn struct {
	header string
	path   string
}

func renderCustomColumns(w io.Writer, items []unstructured.Unstructured, expression string) error {
	columns, err := parseCustomColumns(expression)
	if err != nil {
		return err
	}
	for index, column := range columns {
		if index > 0 {
			_, _ = fmt.Fprint(w, "\t")
		}
		_, _ = fmt.Fprint(w, column.header)
	}
	_, _ = fmt.Fprintln(w)
	for _, item := range items {
		for index, column := range columns {
			if index > 0 {
				_, _ = fmt.Fprint(w, "\t")
			}
			value, valueErr := jsonPathString(item, column.path)
			if valueErr != nil {
				return valueErr
			}
			_, _ = fmt.Fprint(w, value)
		}
		_, _ = fmt.Fprintln(w)
	}
	return nil
}

func parseCustomColumns(expression string) ([]customColumn, error) {
	if strings.TrimSpace(expression) == "" {
		return nil, fmt.Errorf("custom-columns requires a column specification")
	}
	columns := make([]customColumn, 0)
	hasNamespace := false
	for _, raw := range strings.Split(expression, ",") {
		parts := strings.SplitN(raw, ":", 2)
		if len(parts) != 2 || strings.TrimSpace(parts[0]) == "" || strings.TrimSpace(parts[1]) == "" {
			return nil, fmt.Errorf("invalid custom column %q", raw)
		}
		column := customColumn{header: strings.TrimSpace(parts[0]), path: strings.TrimSpace(parts[1])}
		if strings.EqualFold(column.header, "NAMESPACE") || strings.Trim(column.path, "{}") == ".metadata.namespace" {
			hasNamespace = true
		}
		columns = append(columns, column)
	}
	if !hasNamespace {
		columns = append([]customColumn{{header: "NAMESPACE", path: ".metadata.namespace"}}, columns...)
	}
	return columns, nil
}

func jsonPathString(item unstructured.Unstructured, expression string) (string, error) {
	printer, err := printers.NewJSONPathPrinter(withTemplateBraces(expression))
	if err != nil {
		return "", err
	}
	var output strings.Builder
	if err := printer.PrintObj(&item, &output); err != nil {
		return "", err
	}
	return output.String(), nil
}

func withTemplateBraces(expression string) string {
	expression = strings.TrimSpace(expression)
	if strings.HasPrefix(expression, "{") {
		return expression
	}
	return "{" + expression + "}"
}
