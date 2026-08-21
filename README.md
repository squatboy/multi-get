[![Go version](https://img.shields.io/github/go-mod/go-version/squatboy/multi-get?style=flat-square&logo=go&logoColor=white)](https://go.dev/)
[![CI](https://img.shields.io/github/actions/workflow/status/squatboy/multi-get/test.yml?branch=main&style=flat-square&label=CI)](https://github.com/squatboy/multi-get/actions/workflows/test.yml)

# multi-get

kubectl [plugin](https://kubernetes.io/docs/tasks/extend-kubectl/kubectl-plugins/) that lets you query the same Kubernetes resource across multiple namespaces with one kubectl command.

Choose the namespaces explicitly, find them by name, or query every namespace:

```bash
kubectl multi-get get pods -n dev,stage,prod
kubectl multi-get get deploy --find-ns dev
kubectl multi-get get pods -A
```

This multi-namespace lookup is the core feature. The plugin sends the resource query to each selected Namespace and combines the results with the Namespace shown in human-readable output.

Regular kubectl already supports multiple resource kinds in one namespace:

```bash
kubectl get pod,svc,pvc -n dev
```

It also supports all namespaces for many normal list commands. `kubectl-multi-get` keeps those familiar resource expressions as a secondary composition feature, so the following applies each resource query to both namespaces and prints separate blocks:

```bash
kubectl multi-get get pods,services -n dev,stage
```

## Install

### Linux/MacOS

Install the latest macOS/Linux release:

```bash
curl -fsSL https://github.com/squatboy/multi-get/releases/latest/download/install.sh | sh
```

The installer detects the operating system and CPU architecture, verifies the
downloaded archive, and installs `kubectl-multi-get` into `$HOME/.local/bin`.

The installer adds `$HOME/.local/bin` to the detected shell configuration.
Open a new terminal, or reload the shell configuration using the command shown
by the installer.

Verify the installation:

```bash
kubectl multi-get --help
```

### Krew

After the initial plugin registration is merged into the Krew index, install it with:

```bash
kubectl krew install multi
kubectl multi-get get pods -n dev,stage
```

### Local Build

Build the plugin locally:

```bash
make build
export PATH="$PWD/bin:$PATH"
kubectl multi-get get pods -n dev,stage
```

This repository targets Go 1.25.x and Kubernetes client modules v0.35.x.

The plugin reads the same kubeconfig, context, TLS, authentication, and request-timeout flags as kubectl. It calls the Kubernetes API directly and does not invoke kubectl recursively.

## Namespace selection

Exactly one namespace mode is used:

| Command | Behavior |
| --- | --- |
| `kubectl multi-get get pods` | Current context namespace, or `default` |
| `kubectl multi-get get pods -n dev,stage` | Explicit namespaces, in input order |
| `kubectl multi-get get pods --find-ns dev` | Namespace names containing `dev`, sorted by name |
| `kubectl multi-get get pods -A` | All namespaces, sorted by name |

Explicit lists trim whitespace and remove duplicate names while preserving the first occurrence. Empty entries and mixed namespace modes are errors. `--find-ns` trims the search text and matches it literally anywhere in each Namespace name. Empty search text and a search with no matches are errors and do not start resource requests.

## Selectors and output

Selectors are sent to every namespaced List request:

```bash
kubectl multi-get get pods -n dev,stage -l app=backend -o wide
kubectl multi-get get pods -n dev,stage --field-selector status.phase=Running
kubectl multi-get get pods -n dev,stage --sort-by=.metadata.creationTimestamp
```

Supported output formats are `wide`, `json`, `yaml`, `name`, `custom-columns`, `jsonpath`, and `go-template`. Human tables always put `NAMESPACE` first. JSON and YAML for one resource are emitted as one Kubernetes `List` object.

```bash
kubectl multi-get get pods -n dev,stage -o json
kubectl multi-get get pods -n dev,stage -o name
kubectl multi-get get pods -n dev,stage -o 'custom-columns=NAME:.metadata.name,PHASE:.status.phase'
```

Named lookup supports multiple object names for one resource type:

```bash
kubectl multi-get get pod api web -n dev,stage
kubectl multi-get get pod missing -n dev,stage --ignore-not-found
```

Namespace requests and resource requests are sequential. Successful results are printed even when another namespace or resource request fails; any real API error still gives exit code 1.

## Supported and unsupported scope

| Supported | Not supported |
| --- | --- |
| Multiple explicit namespaces | Cluster-scoped resources such as `nodes` |
| `--find-ns` and `-A` | Subresources such as `pods/status` |
| Multiple resource kinds as a cross-namespace composition | `--watch` and `--watch-only` |
| Named lookup and `--ignore-not-found` | `-f/--filename` and `--raw` |
| Label and field selectors | Multi-resource with structured output |
| Table, wide, JSON, YAML, name, and template output | Multi-resource with object names |
| Namespaced built-in resources and namespaced CRDs | Namespace exclude and parallel queries |

The repository includes a Krew manifest template for the initial registration. Until it is
available in the Krew index, use the manual installer above. The plugin does not provide
shell-completion.

## Verification

The regular checks do not mutate a cluster:

```bash
make test
make vet
make build
```

The local E2E uses the current kubeconfig context and creates labeled, uniquely named fixtures only when the mutation guard is explicit:

```bash
E2E_ALLOW_CLUSTER_MUTATION=1 make e2e-local
```

The E2E trap removes only fixtures with its generated prefix and E2E labels.
