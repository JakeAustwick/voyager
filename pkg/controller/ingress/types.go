package ingress

import (
	"os/exec"
	"strings"
	"sync"

	aci "github.com/appscode/k8s-addons/api"
	acs "github.com/appscode/k8s-addons/client/clientset"
	"github.com/appscode/k8s-addons/pkg/stash"
	"github.com/appscode/log"
	"k8s.io/kubernetes/pkg/client/cache"
	clientset "k8s.io/kubernetes/pkg/client/clientset_generated/internalclientset"
	"k8s.io/kubernetes/pkg/cloudprovider"
)

const (
	ServicePrefix    = "lb-svc-"
	ControllerPrefix = "lb-c-"
	DaemonSetPrefix  = "lb-dm-"
	ConfigMapPrefix  = "lb-cm-"

	stickySession = "ingress.appscode.com/stickySession"

	// LB stats options
	StatPort      = 1936
	StatsOn       = "ingress.appscode.com/stats"
	StatsUser     = "ingress.appscode.com/stats.user"
	StatsPassword = "ingress.appscode.com/stats.password"

	// Daemon, Persistent, LoadBalancer
	LBName         = "ingress.appscode.com/name"
	LBType         = "ingress.appscode.com/type"
	LBDaemon       = "Daemon"
	LBLoadBalancer = "LoadBalancer" // default

	// Runs on a specific set of a hosts via DaemonSet. This is needed to work around the issue that master node is registered but not scheduable.
	DaemonHostname = "ingress.appscode.com/daemon.hostname"

	// LoadBalancer mode exposes HAProxy via a type=LoadBalancer service. This is the original version implemented by @sadlil
	// Uses nodeport and Cloud LoadBalancer exists beyond single HAProxy run
	LoadBalancerIP      = "ingress.appscode.com/ip"                   // external_ip or loadbalancer_ip "" or a "ipv4"
	LoadBalancerPersist = "ingress.appscode.com/loadbalancer.persist" // "" or a "ipv4"
)

type annotation map[string]string

func (s annotation) StickySession() bool {
	_, ok := s[stickySession]
	return ok
}

func (s annotation) Stats() bool {
	_, ok := s[StatsOn]
	return ok
}

func (s annotation) StatsUser() string {
	v, _ := s[StatsUser]
	return v
}

func (s annotation) StatsPassword() string {
	v, _ := s[StatsPassword]
	return v
}

func (s annotation) LBType() string {
	if v, ok := s[LBType]; ok {
		return v
	}
	return LBLoadBalancer
}

func (s annotation) DaemonHostname() string {
	v, _ := s[DaemonHostname]
	return v
}

func (s annotation) LoadBalancerIP() string {
	v, _ := s[LoadBalancerIP]
	return v
}

func (s annotation) LoadBalancerPersist() bool {
	v, _ := s[LoadBalancerPersist]
	return strings.ToLower(v) == "true"
}

type EngressController struct {
	// kubernetes client
	KubeClient        clientset.Interface
	ACExtensionClient acs.AppsCodeExtensionInterface
	CloudManager      cloudprovider.Interface

	// Engress object that created or updated.
	Config  *aci.Ingress
	Options *KubeOptions
	// contains all the https host names.
	HostFilter []string

	// parsed ingress.
	Parsed *HAProxyOptions

	// endpoint cache store. contains all endpoints will be
	// search with respect to services.
	Storage       *stash.Storage
	EndpointStore cache.StoreToEndpointsLister

	LoadbalancerImage string

	sync.Mutex

	IngressClass string
}

type KubeOptions struct {
	// name of the cluster the daemon running.
	ClusterName string

	ProviderName string
	// kube options data
	SecretNames []string

	ConfigMapName string
	// contains raw configMap data parsed from the cfg file.
	ConfigData string

	// port list the pods needs and service needs to listen to.
	Ports []int

	LBType              string
	DaemonHostname      string
	LoadBalancerIP      string
	LoadBalancerPersist bool
}

type HAProxyOptions struct {
	Timestamp int64
	// those options are get from annotations. applied globally
	// in all the sections.

	// stick requests to specified servers.
	Sticky  bool
	SSLCert bool

	// open up load balancer stats
	Stats bool
	// Basic auth to lb stats
	StatsUserName string
	StatsPassWord string

	DefaultBackend *Backend
	HttpsService   []*Service
	HttpService    []*Service
	TCPService     []*TCPService
}

type Service struct {
	Name     string
	AclMatch string
	Host     string
	Backends *Backend
}

type TCPService struct {
	Name       string
	Host       string
	Port       string
	SecretName string
	PEMName    string
	Backends   *Backend
}

type Backend struct {
	Name         string      `json:"Name,omitempty"`
	RewriteRules []string    `json:"RewriteRules,omitempty"`
	HeaderRules  []string    `json:"HeaderRules,omitempty"`
	Endpoints    []*Endpoint `json:"Endpoints,omitempty"`
}

type Endpoint struct {
	Name string
	IP   string
	Port string
}

// Loadbalancer image is an almost constant type.
// this will only be set at the runtime but only for once.
// once this is set the value can not be changed.
var loadbalancerImage string

func SetLoadbalancerImage(i string) {
	var once sync.Once
	once.Do(
		func() {
			loadbalancerImage = i
		},
	)
}

func GetLoadbalancerImage() string {
	return loadbalancerImage
}

func fullyQualifiedNodeName(nodeName string) (string, error) {
	if idx := strings.Index(nodeName, "."); idx >= 0 {
		nodeName = nodeName[0:idx]
	}

	// Returns master's fqdn since kubed uses host network
	name, err := exec.Command("hostname", "-f").Output()
	if err != nil {
		return "", err
	}
	fqdn := strings.TrimSpace(string(name))
	log.Debugln("Found fqdn:", fqdn)
	firstDotIndex := strings.Index(fqdn, ".")
	if firstDotIndex >= 0 {
		return nodeName + fqdn[firstDotIndex:], nil
	}
	return nodeName, nil
}
