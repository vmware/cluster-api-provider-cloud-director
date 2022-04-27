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

type Network struct {
	Cni      Cni      `json:"cni,omitempty"`
	Pods     Pods     `json:"pods,omitempty"`
	Services Services `json:"services,omitempty"`
}

type Settings struct {
	OvdcNetwork string  `json:"ovdcNetwork,omitempty"`
	Network     Network `json:"network,omitempty"`
}

type CloudProperties struct {
	Site string `json:"site,omitempty"`
	Org  string `json:"orgName,omitempty"`
	Vdc  string `json:"virtualDataCenterName,omitempty"`
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

type CAPVCDStatus struct {
	Phase                  string            `json:"phase,omitempty"`
	Kubernetes             string            `json:"kubernetes,omitempty"`
	Uid                    string            `json:"uid,omitempty"`
	ClusterAPIStatus       ClusterApiStatus  `json:"clusterApiStatus,omitempty"`
	NodeStatus             map[string]string `json:"nodeStatus,omitempty"`
	CapvcdVersion          string            `json:"capvcdVersion,omitempty"`
	UseAsManagementCluster bool              `json:"useAsManagementCluster,omitempty"`
}
type Status struct {
	CAPVCDStatus      CAPVCDStatus    `json:"capvcd,omitempty"`
	CloudProperties   CloudProperties `json:"cloudProperties,omitempty"`
	PersistentVolumes []string        `json:"persistentVolumes,omitempty"`
	VirtualIPs        []string        `json:"virtualIPs,omitempty"`
	ParentUID         string          `json:"parentUid,omitempty"`
	Cni               VersionedAddon  `json:"cni,omitempty"`
	Cpi               VersionedAddon  `json:"cpi,omitempty"`
	Csi               VersionedAddon  `json:"csi,omitempty"`
	Private           PrivateSection  `json:"private,omitempty"`
}

type ClusterSpec struct {
	//Settings     Settings     `json:"settings"`
	//Topology     Topology     `json:"topology"`
	//Distribution Distribution `json:"distribution"`
	CapiYaml string `json:"capiYaml,omitempty"`
}

type CAPVCDEntity struct {
	Metadata   Metadata    `json:"metadata"`
	Spec       ClusterSpec `json:"spec"`
	ApiVersion string      `json:"apiVersion"`
	Status     Status      `json:"status"`
	Kind       string      `json:"kind"`
}
