package cli

import (
	"fmt"
	"io"

	"github.com/squatboy/multi-get/internal/query"
)

func printQueryErrors(w io.Writer, results []query.QueryResult) int {
	count := 0
	for _, result := range results {
		for _, queryErr := range result.Errors {
			count++
			location := fmt.Sprintf("resource=%s", queryErr.Resource)
			if queryErr.Namespace != "" {
				location += fmt.Sprintf(" namespace=%s", queryErr.Namespace)
			}
			if queryErr.Name != "" {
				location += fmt.Sprintf(" name=%s", queryErr.Name)
			}
			location += fmt.Sprintf(" operation=%s", queryErr.Operation)
			_, _ = fmt.Fprintf(w, "%s: %v\n", location, queryErr.Err)
		}
	}
	return count
}
