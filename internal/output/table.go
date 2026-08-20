package output

import (
	"encoding/json"
	"fmt"
	"io"
	"strings"
	"text/tabwriter"
	"time"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"

	"github.com/squatboy/multi-get/internal/query"
)

type TableOptions struct {
	Wide      bool
	NoHeaders bool
	Multi     bool
}

type tableColumn struct {
	definition metav1.TableColumnDefinition
	source     int
}

func RenderTable(w io.Writer, results []query.QueryResult, opts TableOptions) error {
	for index, result := range results {
		if opts.Multi {
			if index > 0 {
				if _, err := fmt.Fprintln(w); err != nil {
					return err
				}
			}
			if _, err := fmt.Fprintf(w, "%s:\n", result.Resource.Resource); err != nil {
				return err
			}
		}
		if err := renderOneTable(w, result, opts); err != nil {
			return err
		}
	}
	return nil
}

func renderOneTable(w io.Writer, result query.QueryResult, opts TableOptions) error {
	if !result.TableFallback && len(result.TablePages) > 0 {
		if ok, err := renderServerTable(w, result, opts); err != nil {
			return err
		} else if ok {
			return nil
		}
	}
	return renderGenericTable(w, result, opts.NoHeaders)
}

func renderServerTable(w io.Writer, result query.QueryResult, opts TableOptions) (bool, error) {
	columns := result.TablePages[0].Table.ColumnDefinitions
	for _, page := range result.TablePages[1:] {
		if !sameColumns(columns, page.Table.ColumnDefinitions) {
			return false, nil
		}
	}
	selected := make([]tableColumn, 0, len(columns)+1)
	selected = append(selected, tableColumn{definition: metav1.TableColumnDefinition{Name: "NAMESPACE", Priority: 0}, source: -1})
	for source, column := range columns {
		if strings.EqualFold(column.Name, "NAMESPACE") {
			continue
		}
		if !opts.Wide && column.Priority > 0 {
			continue
		}
		selected = append(selected, tableColumn{definition: column, source: source})
	}

	rows := make(map[string][]interface{})
	for _, page := range result.TablePages {
		for _, row := range page.Table.Rows {
			object, err := tableRowObject(row)
			if err != nil {
				return false, nil
			}
			namespace := object.GetNamespace()
			if namespace == "" {
				namespace = page.Namespace
			}
			rows[objectKey(namespace, object.GetName())] = row.Cells
		}
	}

	tw := tabwriter.NewWriter(w, 0, 4, 2, ' ', 0)
	if !opts.NoHeaders {
		writeTableLine(tw, headerNames(selected))
	}
	for _, item := range result.Items {
		cells, ok := rows[objectKey(item.GetNamespace(), item.GetName())]
		if !ok {
			return false, nil
		}
		line := make([]string, 0, len(selected))
		line = append(line, item.GetNamespace())
		for _, column := range selected[1:] {
			if column.source < 0 || column.source >= len(cells) {
				line = append(line, "<none>")
				continue
			}
			line = append(line, formatCell(cells[column.source]))
		}
		writeTableLine(tw, line)
	}
	return true, tw.Flush()
}

func renderGenericTable(w io.Writer, result query.QueryResult, noHeaders bool) error {
	tw := tabwriter.NewWriter(w, 0, 4, 2, ' ', 0)
	if !noHeaders {
		writeTableLine(tw, []string{"NAMESPACE", "NAME", "AGE"})
	}
	for _, item := range result.Items {
		writeTableLine(tw, []string{item.GetNamespace(), item.GetName(), formatAge(item.GetCreationTimestamp().Time)})
	}
	return tw.Flush()
}

func sameColumns(left, right []metav1.TableColumnDefinition) bool {
	if len(left) != len(right) {
		return false
	}
	for i := range left {
		if left[i] != right[i] {
			return false
		}
	}
	return true
}

func headerNames(columns []tableColumn) []string {
	result := make([]string, 0, len(columns))
	for _, column := range columns {
		result = append(result, strings.ToUpper(column.definition.Name))
	}
	return result
}

func writeTableLine(w io.Writer, values []string) {
	_, _ = fmt.Fprintln(w, strings.Join(values, "\t"))
}

func formatCell(value interface{}) string {
	if value == nil {
		return "<none>"
	}
	if text, ok := value.(string); ok {
		return text
	}
	data, err := json.Marshal(value)
	if err == nil && len(data) > 0 {
		return string(data)
	}
	return fmt.Sprint(value)
}

func tableRowObject(row metav1.TableRow) (unstructured.Unstructured, error) {
	var object unstructured.Unstructured
	if len(row.Object.Raw) > 0 {
		if err := json.Unmarshal(row.Object.Raw, &object); err != nil {
			return object, err
		}
		return object, nil
	}
	if row.Object.Object != nil {
		data, err := json.Marshal(row.Object.Object)
		if err != nil {
			return object, err
		}
		if err := json.Unmarshal(data, &object); err != nil {
			return object, err
		}
		return object, nil
	}
	return object, fmt.Errorf("Table row did not include its object")
}

func objectKey(namespace, name string) string { return namespace + "\x00" + name }

func formatAge(created time.Time) string {
	if created.IsZero() {
		return "<unknown>"
	}
	age := time.Since(created)
	if age < 0 {
		age = 0
	}
	switch {
	case age >= 24*time.Hour:
		return fmt.Sprintf("%dd", int(age/(24*time.Hour)))
	case age >= time.Hour:
		return fmt.Sprintf("%dh", int(age/time.Hour))
	case age >= time.Minute:
		return fmt.Sprintf("%dm", int(age/time.Minute))
	default:
		return fmt.Sprintf("%ds", int(age/time.Second))
	}
}
