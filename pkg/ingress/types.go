package ingress

// ObjectMeta captures common Kubernetes metadata.
type ObjectMeta struct {
	Name              string            `json:"name"`
	Namespace         string            `json:"namespace"`
	Labels            map[string]string `json:"labels"`
	Annotations       map[string]string `json:"annotations"`
	ResourceVersion   string            `json:"resourceVersion"`
	CreationTimestamp string            `json:"creationTimestamp"`
}

// IngressList models a collection of Ingress objects returned by K8s API.
type IngressList struct {
	Kind       string    `json:"kind"`
	APIVersion string    `json:"apiVersion"`
	Items      []Ingress `json:"items"`
}

// Ingress models networking.k8s.io/v1 Ingress resource.
type Ingress struct {
	Kind       string      `json:"kind"`
	APIVersion string      `json:"apiVersion"`
	Metadata   ObjectMeta  `json:"metadata"`
	Spec       IngressSpec `json:"spec"`
}

// IngressSpec models Ingress rules and TLS settings.
type IngressSpec struct {
	IngressClassName *string       `json:"ingressClassName"`
	Rules            []IngressRule `json:"rules"`
	TLS              []IngressTLS  `json:"tls"`
}

// IngressRule models per-host routing rules.
type IngressRule struct {
	Host string                `json:"host"`
	HTTP *HTTPIngressRuleValue `json:"http"`
}

// HTTPIngressRuleValue captures paths under an HTTP rule.
type HTTPIngressRuleValue struct {
	Paths []HTTPIngressPath `json:"paths"`
}

// HTTPIngressPath models a subpath routing rule.
type HTTPIngressPath struct {
	Path     string         `json:"path"`
	PathType string         `json:"pathType"` // Prefix, Exact, ImplementationSpecific
	Backend  IngressBackend `json:"backend"`
}

// IngressBackend models the backend Service or Resource target.
type IngressBackend struct {
	Service *IngressServiceBackend `json:"service"`
}

// IngressServiceBackend captures Service name and port.
type IngressServiceBackend struct {
	Name string             `json:"name"`
	Port ServiceBackendPort `json:"port"`
}

// ServiceBackendPort models port selection by number or name.
type ServiceBackendPort struct {
	Name   string `json:"name"`
	Number int    `json:"number"`
}

// IngressTLS models TLS configuration and Secret mapping.
type IngressTLS struct {
	Hosts      []string `json:"hosts"`
	SecretName string   `json:"secretName"`
}

// Endpoints models a Kubernetes Endpoints object for a Service.
type Endpoints struct {
	Metadata ObjectMeta       `json:"metadata"`
	Subsets  []EndpointSubset `json:"subsets"`
}

// EndpointSubset captures ready IP addresses and ports.
type EndpointSubset struct {
	Addresses []EndpointAddress `json:"addresses"`
	Ports     []EndpointPort    `json:"ports"`
}

// EndpointAddress models a pod IP address.
type EndpointAddress struct {
	IP       string `json:"ip"`
	Hostname string `json:"hostname"`
}

// EndpointPort models an exposed endpoint port.
type EndpointPort struct {
	Name     string `json:"name"`
	Port     int    `json:"port"`
	Protocol string `json:"protocol"`
}

// Secret models a Kubernetes Secret object (e.g. kubernetes.io/tls).
type Secret struct {
	Metadata ObjectMeta        `json:"metadata"`
	Type     string            `json:"type"`
	Data     map[string]string `json:"data"` // Base64 encoded values
}

// K8sWatchEvent captures HTTP streaming watch event objects.
type K8sWatchEvent struct {
	Type   string         `json:"type"` // ADDED, MODIFIED, DELETED
	Object jsonRawMessage `json:"object"`
}

type jsonRawMessage []byte

func (m jsonRawMessage) MarshalJSON() ([]byte, error) {
	if m == nil {
		return []byte("null"), nil
	}
	return m, nil
}

func (m *jsonRawMessage) UnmarshalJSON(data []byte) error {
	if m == nil {
		return nil
	}
	*m = append((*m)[:0], data...)
	return nil
}
