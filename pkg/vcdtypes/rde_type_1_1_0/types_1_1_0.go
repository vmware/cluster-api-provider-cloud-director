package rde_type_1_1_0

type Metadata struct {
	Name string `json:"name,omitempty"`
	Org  string `json:"orgName,omitempty"`
	Vdc  string `json:"virtualDataCenterName,omitempty"`
	Site string `json:"site,omitempty"`
}

type ControlPlane struct {
	SizingClass  string `json:"sizingClass,omitempty"`
	Count        int32  `json:"count,omitempty"`
	TemplateName string `json:"templateName,omitempty"`
}

type Workers struct {
	SizingClass  string `json:"sizingClass,omitempty"`
	Count        int32  `json:"count,omitempty"`
	TemplateName string `json:"templateName,omitempty"`
}

type Distribution struct {
	Version string `json:"version,omitempty"`
}

type Topology struct {
	ControlPlane []ControlPlane `json:"controlPlane,omitempty"`
	Workers      []Workers      `json:"workers,omitempty"`
}

type Cni struct {
	Name string `json:"name,omitempty"`
}

type Pods struct {
	CidrBlocks []string `json:"cidrBlocks,omitempty"`
}

type Services struct {
	CidrBlocks []string `json:"cidrBlocks,omitempty"`
}

type VCDProperties struct {
	Site        string `json:"site,omitempty"`
	Org         string `json:"orgName,omitempty"`
	Vdc         string `json:"virtualDataCenterName,omitempty"`
	OvdcNetwork string `json:"ovdcNetworkName,omitempty"`
}

type ApiEndpoints struct {
	Host string `json:"host,omitempty"`
	Port int32  `json:"port,omitempty"`
}

type ClusterApiStatus struct {
	Phase        string         `json:"phase,omitempty"`
	ApiEndpoints []ApiEndpoints `json:"apiEndpoints,omitempty"`
}

type VersionedAddon struct {
	Name    string `json:"name,omitempty"`
	Version string `json:"version,omitempty"`
}

type PrivateSection struct {
	KubeConfig string `json:"kubeConfig,omitempty"`
}

type K8sNetwork struct {
	Cni      Cni      `json:"cni,omitempty"`
	Pods     Pods     `json:"pods,omitempty"`
	Services Services `json:"services,omitempty"`
}

type ClusterResource struct {
	Name    string `json:"name,omitempty"`
	Version string `json:"version,omitempty"`
}

type VCDResource struct {
	Type              string                 `json:"type,omitempty"`
	ID                string                 `json:"id,omitempty"`
	Name              string                 `json:"name,omitempty"`
	AdditionalDetails map[string]interface{} `json:"additionalDetails,omitempty"`
}

type CAPVCDStatus struct {
	Phase                   string            `json:"phase,omitempty"`
	Kubernetes              string            `json:"kubernetes,omitempty"`
	Uid                     string            `json:"uid,omitempty"`
	ClusterAPIStatus        ClusterApiStatus  `json:"clusterApiStatus,omitempty"`
	NodeStatus              map[string]string `json:"nodeStatus,omitempty"`
	CapvcdVersion           string            `json:"capvcdVersion,omitempty"`
	UsedAsManagementCluster bool              `json:"userIntendsToUseItAsAManagementCluster,omitempty"`
	Errors                  []string          `json:"errors,omitempty"`
	K8sNetwork              K8sNetwork        `json:"k8sNetwork,omitempty"`
	ParentUID               string            `json:"parentUid,omitempty"`
	ClusterResourceSet      []ClusterResource `json:"clusterResourceSet,omitempty"`
	VcdProperties           VCDProperties     `json:"vcdProperties,omitempty"`
	Private                 PrivateSection    `json:"private,omitempty"`
	VCDResourceSet          []VCDResource     `json:"VCDResourceSet,omitempty"`
}

type Status struct {
	CAPVCDStatus      CAPVCDStatus `json:"capvcd,omitempty"`
	PersistentVolumes []string     `json:"persistentVolumes,omitempty"`
	VirtualIPs        []string     `json:"virtualIPs,omitempty"`
}

type ClusterSpec struct {
	CapiYaml string `json:"capiYaml,omitempty"`
}

type CAPVCDEntity struct {
	Metadata   Metadata    `json:"metadata"`
	Spec       ClusterSpec `json:"spec"`
	ApiVersion string      `json:"apiVersion"`
	Status     Status      `json:"status"`
	Kind       string      `json:"kind"`
}
