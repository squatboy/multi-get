package cli

import "github.com/spf13/pflag"

// countedString remembers whether a flag was supplied more than once. pflag
// otherwise silently accepts repeated scalar flags, which is not part of the
// plugin's namespace contract.
type countedString struct {
	value string
	count int
}

func (v *countedString) Set(value string) error {
	v.value = value
	v.count++
	return nil
}

func (v *countedString) String() string { return v.value }

func (v *countedString) Type() string { return "string" }

func addNamespaceFlags(flags *pflag.FlagSet) (*countedString, *countedString, *bool) {
	explicit := &countedString{}
	find := &countedString{}
	all := false
	flags.VarP(explicit, "namespace", "n", "Comma-separated namespaces to query")
	flags.Var(find, "find-ns", "Select namespaces whose name contains the given text")
	flags.BoolVarP(&all, "all-namespaces", "A", false, "Query all namespaces")
	return explicit, find, &all
}
