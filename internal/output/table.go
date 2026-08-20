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

const (
	grayStart  = "\x1b[90m"
	boldStart  = "\x1b[1m"
	colorReset = "\x1b[0m"
)

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

	rowCells := make(map[string][]interface{})
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
			rowCells[objectKey(namespace, object.GetName())] = row.Cells
		}
	}

	rows := make([][]string, 0, len(result.Items))
	for _, item := range result.Items {
		cells, ok := rowCells[objectKey(item.GetNamespace(), item.GetName())]
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
		rows = append(rows, line)
	}
	return true, renderAlignedTable(w, headerNames(selected), rows, opts.NoHeaders)
}

func renderGenericTable(w io.Writer, result query.QueryResult, noHeaders bool) error {
	rows := make([][]string, 0, len(result.Items))
	for _, item := range result.Items {
		rows = append(rows, []string{item.GetNamespace(), item.GetName(), formatAge(item.GetCreationTimestamp().Time)})
	}
	return renderAlignedTable(w, []string{"NAMESPACE", "NAME", "AGE"}, rows, noHeaders)
}

func renderAlignedTable(w io.Writer, headers []string, rows [][]string, noHeaders bool) error {
	var table strings.Builder
	tw := tabwriter.NewWriter(&table, 0, 4, 2, ' ', 0)
	if !noHeaders {
		writeTableLine(tw, headers)
	}
	for _, row := range rows {
		writeTableLine(tw, row)
	}
	if err := tw.Flush(); err != nil {
		return err
	}
	formatted := table.String()
	if !noHeaders {
		formatted = boldHeader(formatted)
	}
	_, err := io.WriteString(w, colorizeNamespaceValues(formatted, !noHeaders))
	return err
}

func boldHeader(table string) string {
	lineEnd := strings.IndexByte(table, '\n')
	if lineEnd < 0 {
		return boldStart + table + colorReset
	}
	return boldStart + table[:lineEnd] + colorReset + table[lineEnd:]
}

func colorizeNamespaceValues(table string, skipHeader bool) string {
	var colored strings.Builder
	for index, line := range strings.SplitAfter(table, "\n") {
		if line == "" {
			continue
		}
		if skipHeader && index == 0 {
			colored.WriteString(line)
			continue
		}
		content := strings.TrimSuffix(line, "\n")
		fieldEnd := strings.IndexAny(content, " \t")
		if fieldEnd <= 0 {
			colored.WriteString(line)
			continue
		}
		colored.WriteString(grayStart)
		colored.WriteString(content[:fieldEnd])
		colored.WriteString(colorReset)
		colored.WriteString(content[fieldEnd:])
		if strings.HasSuffix(line, "\n") {
			colored.WriteByte('\n')
		}
	}
	return colored.String()
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
