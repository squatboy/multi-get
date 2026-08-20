package cli

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"strings"

	"github.com/spf13/cobra"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/cli-runtime/pkg/genericclioptions"

	"github.com/squatboy/multi-get/internal/kube"
	"github.com/squatboy/multi-get/internal/namespace"
	"github.com/squatboy/multi-get/internal/output"
	"github.com/squatboy/multi-get/internal/query"
)

type getFlags struct {
	explicit       *countedString
	findNS         *countedString
	allNamespaces  *bool
	selector       string
	fieldSelector  string
	sortBy         string
	output         string
	noHeaders      bool
	ignoreNotFound bool
}

func newGetCommand(ctx context.Context, configFlags *genericclioptions.ConfigFlags, stdout, stderr io.Writer) *cobra.Command {
	flags := &getFlags{}
	cmd := &cobra.Command{
		Use:   "get RESOURCE[,RESOURCE...] [NAME ...]",
		Short: "Get namespaced resources across one or more namespaces",
		Args: func(_ *cobra.Command, args []string) error {
			if len(args) == 0 {
				return errors.New("resource argument is required")
			}
			return nil
		},
		RunE: func(_ *cobra.Command, args []string) error {
			return runGet(ctx, configFlags, flags, args, stdout, stderr)
		},
	}
	flags.explicit, flags.findNS, flags.allNamespaces = addNamespaceFlags(cmd.Flags())
	cmd.Flags().StringVarP(&flags.selector, "selector", "l", "", "Selector (label query) to filter resources")
	cmd.Flags().StringVar(&flags.fieldSelector, "field-selector", "", "Selector (field query) to filter resources")
	cmd.Flags().StringVar(&flags.sortBy, "sort-by", "", "Sort the result by a JSONPath expression")
	cmd.Flags().StringVarP(&flags.output, "output", "o", "", "Output format: wide, json, yaml, name, custom-columns, jsonpath, or go-template")
	cmd.Flags().BoolVar(&flags.noHeaders, "no-headers", false, "Do not print column headers")
	cmd.Flags().BoolVar(&flags.ignoreNotFound, "ignore-not-found", false, "Treat missing named objects as successful")
	return cmd
}

func runGet(ctx context.Context, configFlags *genericclioptions.ConfigFlags, flags *getFlags, args []string, stdout, stderr io.Writer) error {
	if flags.explicit.count > 1 {
		return errors.New("-n/--namespace may be specified only once")
	}
	if flags.findNS.count > 1 {
		return errors.New("--find-ns may be specified only once")
	}

	resourceInputs, err := parseResourceInputs(args[0])
	if err != nil {
		return err
	}
	names := append([]string(nil), args[1:]...)
	if len(resourceInputs) > 1 && len(names) > 0 {
		return errors.New("object names cannot be used with multiple resources")
	}
	format, expression, err := parseOutput(flags.output)
	if err != nil {
		return err
	}
	multiInput := len(resourceInputs) > 1
	if multiInput && !isMultiOutput(format) {
		return fmt.Errorf("-o %s is not supported with multiple resources", format)
	}
	if len(names) > 0 && (flags.selector != "" || flags.fieldSelector != "" || flags.sortBy != "") {
		return errors.New("selectors and --sort-by cannot be used with named lookup")
	}
	outputOptions := output.Options{Format: format, Expression: expression, NoHeaders: flags.noHeaders, Multi: multiInput}
	if err := output.Validate(outputOptions); err != nil {
		return err
	}
	if flags.sortBy != "" {
		if err := query.ValidateSortBy(flags.sortBy); err != nil {
			return fmt.Errorf("invalid --sort-by expression: %w", err)
		}
	}

	clients, err := kube.NewClients(configFlags)
	if err != nil {
		return err
	}
	currentNamespace := ""
	if flags.explicit.count == 0 && flags.findNS.count == 0 && !*flags.allNamespaces {
		currentNamespace, err = kube.CurrentNamespace(configFlags)
		if err != nil {
			return fmt.Errorf("read current context namespace: %w", err)
		}
	}
	namespaces, err := namespace.Resolve(ctx, namespace.Options{
		Explicit:    flags.explicit.value,
		FindNS:      flags.findNS.value,
		All:         *flags.allNamespaces,
		Current:     currentNamespace,
		ExplicitSet: flags.explicit.count > 0,
		FindNSSet:   flags.findNS.count > 0,
	}, clients)
	if err != nil {
		return err
	}

	resolver := kube.Resolver{Discovery: clients.Discovery, Mapper: clients.RESTMapper}
	specs := make([]query.ResourceSpec, 0, len(resourceInputs))
	seen := make(map[schema.GroupVersionResource]struct{}, len(resourceInputs))
	for _, input := range resourceInputs {
		spec, resolveErr := resolver.Resolve(input)
		if resolveErr != nil {
			return resolveErr
		}
		if _, ok := seen[spec.GVR]; ok {
			continue
		}
		seen[spec.GVR] = struct{}{}
		specs = append(specs, spec)
	}
	if len(specs) == 0 {
		return errors.New("at least one resource is required")
	}
	if len(specs) > 1 && len(names) > 0 {
		return errors.New("object names cannot be used with multiple canonical resources")
	}
	outputOptions.Multi = len(specs) > 1

	useTable := format == "" || format == "wide"
	results := make([]query.QueryResult, 0, len(specs))
	for _, spec := range specs {
		results = append(results, query.Run(ctx, clients.Resources, spec, namespaces, query.QueryOptions{
			Names:          names,
			LabelSelector:  flags.selector,
			FieldSelector:  flags.fieldSelector,
			SortBy:         flags.sortBy,
			UseTable:       useTable,
			IgnoreNotFound: flags.ignoreNotFound,
		}))
	}

	var rendered bytes.Buffer
	if err := output.Render(&rendered, results, outputOptions); err != nil {
		return err
	}
	if _, err := io.Copy(stdout, &rendered); err != nil {
		return err
	}
	errorCount := printQueryErrors(stderr, results)
	if errorCount > 0 {
		return fmt.Errorf("%d query operation(s) failed", errorCount)
	}
	return nil
}

func parseResourceInputs(expression string) ([]string, error) {
	parts := strings.Split(expression, ",")
	result := make([]string, 0, len(parts))
	for _, part := range parts {
		part = strings.TrimSpace(part)
		if part == "" {
			return nil, errors.New("resource list contains an empty resource")
		}
		result = append(result, part)
	}
	return result, nil
}

func parseOutput(value string) (string, string, error) {
	value = strings.TrimSpace(value)
	if value == "" || value == "wide" || value == "json" || value == "yaml" || value == "name" {
		return value, "", nil
	}
	for _, format := range []string{"custom-columns", "jsonpath", "go-template"} {
		prefix := format + "="
		if strings.HasPrefix(value, prefix) {
			expression := strings.TrimPrefix(value, prefix)
			if expression == "" {
				return "", "", fmt.Errorf("-o %s requires an expression", format)
			}
			return format, expression, nil
		}
	}
	return "", "", fmt.Errorf("unsupported output format %q", value)
}

func isMultiOutput(format string) bool {
	return format == "" || format == "wide" || format == "name"
}
