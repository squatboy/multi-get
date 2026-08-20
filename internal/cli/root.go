package cli

import (
	"context"
	"fmt"
	"io"

	"github.com/spf13/cobra"
	"k8s.io/cli-runtime/pkg/genericclioptions"
)

// Run executes the plugin and returns a process exit code.
func Run(ctx context.Context, args []string, stdout, stderr io.Writer) int {
	cmd := NewRootCommand(ctx, stdout, stderr)
	cmd.SetArgs(args)
	if err := cmd.Execute(); err != nil {
		_, _ = fmt.Fprintln(stderr, err)
		return 1
	}
	return 0
}

// NewRootCommand creates the kubectl-multi command tree.
func NewRootCommand(ctx context.Context, stdout, stderr io.Writer) *cobra.Command {
	configFlags := genericclioptions.NewConfigFlags(false)
	// -n/--namespace belongs to this plugin and means a comma-separated list.
	// The kubeconfig namespace is read through the raw loader when no plugin
	// namespace selector is supplied.
	configFlags.Namespace = nil

	root := &cobra.Command{
		Use:           "kubectl-multi",
		Short:         "Query namespaced Kubernetes resources across namespaces",
		SilenceErrors: true,
		SilenceUsage:  true,
	}
	root.CompletionOptions.DisableDefaultCmd = true
	configFlags.AddFlags(root.PersistentFlags())
	root.AddCommand(newGetCommand(ctx, configFlags, stdout, stderr))
	return root
}
