package output

import (
	"fmt"
	"io"
	"strings"

	"k8s.io/cli-runtime/pkg/printers"

	"github.com/squatboy/multi-get/internal/query"
)

type Options struct {
	Format     string
	Expression string
	NoHeaders  bool
	Multi      bool
}

func Validate(opts Options) error {
	if opts.Multi && !isMultiFormat(opts.Format) {
		return fmt.Errorf("structured output %s is not supported with multiple resources", opts.Format)
	}
	switch opts.Format {
	case "", "wide", "name", "json", "yaml":
		return nil
	case "custom-columns":
		_, err := parseCustomColumns(opts.Expression)
		return err
	case "jsonpath":
		_, err := printers.NewJSONPathPrinter(withTemplateBraces(opts.Expression))
		return err
	case "go-template":
		if strings.TrimSpace(opts.Expression) == "" {
			return fmt.Errorf("go-template requires an expression")
		}
		_, err := printers.NewGoTemplatePrinter([]byte(opts.Expression))
		return err
	default:
		return fmt.Errorf("unknown output format %q", opts.Format)
	}
}

func Render(w io.Writer, results []query.QueryResult, opts Options) error {
	switch opts.Format {
	case "", "wide":
		return RenderTable(w, results, TableOptions{Wide: opts.Format == "wide", NoHeaders: opts.NoHeaders, Multi: opts.Multi})
	case "name":
		return RenderName(w, results)
	case "json", "yaml", "custom-columns", "jsonpath", "go-template":
		if len(results) != 1 {
			return fmt.Errorf("structured output -o %s is not supported with multiple resources", opts.Format)
		}
		return RenderStructured(w, results[0], opts.Format, opts.Expression)
	default:
		return fmt.Errorf("unknown output format %q", opts.Format)
	}
}

func isMultiFormat(format string) bool {
	return format == "" || format == "wide" || format == "name"
}
