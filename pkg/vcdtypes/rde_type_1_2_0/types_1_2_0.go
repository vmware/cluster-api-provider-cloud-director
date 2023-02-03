package rde_type_1_2_0

import "github.com/vmware/cloud-provider-for-cloud-director/pkg/vcdsdk"

const (
	CapvcdRDETypeVersion = "1.2.0"
)

type Metadata struct {
	Name string `json:"name,omitempty"`
	Vdc  string `json:"virtualDataCenterName,omitempty"`
	Org  string `json:"orgName,omitempty"`
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

type Ovdc struct {
	Name        string `json:"name,omitempty"`
	ID          string `json:"id,omitempty"`
	OvdcNetwork string `json:"ovdcNetworkName,omitempty"`
}

type Org struct {
	Name string `json:"name,omitempty"`
	ID   string `json:"id,omitempty"`
}

type VCDProperties struct {
	Site string `json:"site,omitempty"`
	Ovdc []Ovdc `json:"orgVdcs,omitempty"`
	Org  []Org  `json:"organizations,omitempty"`
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

type NodePool struct {
	Name              string            `json:"name,omitempty"`
	SizingPolicy      string            `json:"sizingPolicy,omitempty"`
	PlacementPolicy   string            `json:"placementPolicy,omitempty"`
	DiskSizeMb        int32             `json:"diskSizeMb,omitempty"`
	NvidiaGpuEnabled  bool              `json:"nvidiaGpuEnabled,omitempty"`
	StorageProfile    string            `json:"storageProfile,omitempty"`
	DesiredReplicas   int32             `json:"desiredReplicas"`
	AvailableReplicas int32             `json:"availableReplicas"`
	NodeStatus        map[string]string `json:"nodeStatus,omitempty"`
}

type ClusterResourceSetBinding struct {
	ClusterResourceSetName string `json:"clusterResourceSetName,omitempty"`
	Kind                   string `json:"kind,omitempty"`
	Name                   string `json:"name,omitempty"`
	Applied                bool   `json:"applied,omitempty"`
	LastAppliedTime        string `json:"lastAppliedTime,omitempty"`
}

type K8sInfo struct {
	TkgVersion string `json:"tkgVersion,omitempty"`
	K8sVersion string `json:"kubernetesVersion,omitempty"`
}

type Upgrade struct {
	Current  *K8sInfo `json:"current"`
	Previous *K8sInfo `json:"previous"`
	Ready    bool     `json:"ready"`
}

type CAPVCDStatus struct {
	Phase                      string                      `json:"phase,omitempty"`
	Kubernetes                 string                      `json:"kubernetes,omitempty"`
	Uid                        string                      `json:"uid,omitempty"`
	ClusterAPIStatus           ClusterApiStatus            `json:"clusterApiStatus,omitempty"`
	NodePool                   []NodePool                  `json:"nodePool,omitempty"`
	CapvcdVersion              string                      `json:"capvcdVersion,omitempty"`
	UseAsManagementCluster     bool                        `json:"useAsManagementCluster,omitempty"`
	ErrorSet                   []vcdsdk.BackendError       `json:"errorSet,omitempty"`
	EventSet                   []vcdsdk.BackendEvent       `json:"eventSet,omitempty"`
	K8sNetwork                 K8sNetwork                  `json:"k8sNetwork,omitempty"`
	ParentUID                  string                      `json:"parentUid,omitempty"`
	ClusterResourceSet         []ClusterResource           `json:"clusterResourceSet,omitempty"`
	VcdProperties              VCDProperties               `json:"vcdProperties,omitempty"`
	Private                    PrivateSection              `json:"private,omitempty"`
	VCDResourceSet             []VCDResource               `json:"vcdResourceSet,omitempty"`
	CapiStatusYaml             string                      `json:"capiStatusYaml,omitempty"`
	ClusterResourceSetBindings []ClusterResourceSetBinding `json:"clusterResourceSetBindings,omitempty"`
	CreatedByVersion           string                      `json:"createdByVersion"`
	Upgrade                    Upgrade                     `json:"upgrade,omitempty"`
}

type Status struct {
	CAPVCDStatus CAPVCDStatus `json:"capvcd,omitempty"`
}

type CAPVCDSpec struct {
	CapiYaml string `json:"capiYaml"`
}

type CAPVCDEntity struct {
	Metadata   Metadata   `json:"metadata"`
	Spec       CAPVCDSpec `json:"spec"`
	ApiVersion string     `json:"apiVersion"`
	Status     Status     `json:"status"`
	Kind       string     `json:"kind"`
}
