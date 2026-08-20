package kube

import (
	"fmt"
	"net/url"

	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/serializer"
	"k8s.io/cli-runtime/pkg/genericclioptions"
	"k8s.io/client-go/discovery"
	"k8s.io/client-go/dynamic"
	corev1 "k8s.io/client-go/kubernetes/typed/core/v1"
	"k8s.io/client-go/rest"
)

// Clients contains the small set of Kubernetes clients used by the plugin.
type Clients struct {
	Dynamic         dynamic.Interface
	Discovery       discovery.CachedDiscoveryInterface
	RESTMapper      meta.RESTMapper
	NamespaceClient corev1.NamespaceInterface
	Resources       *ResourceClient
}

// NewClients builds clients directly from kubectl-compatible connection flags.
func NewClients(flags *genericclioptions.ConfigFlags) (*Clients, error) {
	config, err := flags.ToRESTConfig()
	if err != nil {
		return nil, fmt.Errorf("load Kubernetes client config: %w", err)
	}
	dynamicClient, err := dynamic.NewForConfig(config)
	if err != nil {
		return nil, fmt.Errorf("create dynamic client: %w", err)
	}
	discoveryClient, err := flags.ToDiscoveryClient()
	if err != nil {
		return nil, fmt.Errorf("create discovery client: %w", err)
	}
	restMapper, err := flags.ToRESTMapper()
	if err != nil {
		return nil, fmt.Errorf("create REST mapper: %w", err)
	}
	coreClient, err := corev1.NewForConfig(config)
	if err != nil {
		return nil, fmt.Errorf("create core client: %w", err)
	}
	tableClient, err := newTableClient(config)
	if err != nil {
		return nil, fmt.Errorf("create table client: %w", err)
	}

	return &Clients{
		Dynamic:         dynamicClient,
		Discovery:       discoveryClient,
		RESTMapper:      restMapper,
		NamespaceClient: coreClient.Namespaces(),
		Resources:       &ResourceClient{Dynamic: dynamicClient, Table: tableClient, Namespaces: coreClient.Namespaces()},
	}, nil
}

func newTableClient(config *rest.Config) (*rest.RESTClient, error) {
	baseURL, err := url.Parse(config.Host)
	if err != nil {
		return nil, err
	}
	httpClient, err := rest.HTTPClientFor(config)
	if err != nil {
		return nil, err
	}

	scheme := runtime.NewScheme()
	if err := metav1.AddMetaToScheme(scheme); err != nil {
		return nil, err
	}
	negotiatedSerializer := serializer.NewCodecFactory(scheme).WithoutConversion()
	groupVersion := metav1.SchemeGroupVersion
	content := rest.ClientContentConfig{
		AcceptContentTypes: "application/json;as=Table;g=meta.k8s.io;v=v1, application/json;as=Table;g=meta.k8s.io;v=v1beta1, application/json",
		ContentType:        "application/json",
		GroupVersion:       groupVersion,
		Negotiator:         runtime.NewClientNegotiator(negotiatedSerializer, groupVersion),
	}
	return rest.NewRESTClient(baseURL, "", content, config.RateLimiter, httpClient)
}

// CurrentNamespace reads the namespace from the active kubeconfig context.
func CurrentNamespace(flags *genericclioptions.ConfigFlags) (string, error) {
	namespace, _, err := flags.ToRawKubeConfigLoader().Namespace()
	if err != nil {
		return "", err
	}
	return namespace, nil
}
