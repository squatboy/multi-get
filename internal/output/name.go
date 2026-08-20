package output

import (
	"fmt"
	"io"
	"strings"

	"github.com/squatboy/multi-get/internal/query"
)

func RenderName(w io.Writer, results []query.QueryResult) error {
	for _, result := range results {
		kind := strings.ToLower(result.Resource.Kind)
		for _, item := range result.Items {
			if _, err := fmt.Fprintf(w, "%s/%s/%s\n", kind, item.GetNamespace(), item.GetName()); err != nil {
				return err
			}
		}
	}
	return nil
}
