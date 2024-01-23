/*
Copyright 2021.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package v1beta3

import (
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	clusterv1 "sigs.k8s.io/cluster-api/api/v1beta1"
)

const (
	// ClusterFinalizer allows DockerClusterReconciler to clean up resources associated with DockerCluster before
	// removing it from the apiserver.
	ClusterFinalizer = "vcdcluster.infrastructure.cluster.x-k8s.io"
)

// APIEndpoint represents a reachable Kubernetes API endpoint.
type APIEndpoint struct {
	// Host is the hostname on which the API server is serving.
	Host string `json:"host"`

	// Port is the port on which the API server is serving.
	Port int `json:"port"`
}
type UserCredentialsContext struct {
	Username     string              `json:"username,omitempty"`
	Password     string              `json:"password,omitempty"`
	RefreshToken string              `json:"refreshToken,omitempty"`
	SecretRef    *v1.SecretReference `json:"secretRef,omitempty"`
}

// VCDResources stores the latest ID and name of VCD resources for specific resource types.
type VCDResources []VCDResource

// VCDResourceMap provides a structured way to store and retrieve information about VCD resources
type VCDResourceMap struct {
	Ovdcs VCDResources `json:"ovdcs,omitempty"`
}

// VCDResource restores the data structure for some VCD Resources
type VCDResource struct {
	Type string `json:"type,omitempty"`
	ID   string `json:"id"`
	Name string `json:"name"`
}

// ProxyConfig defines HTTP proxy environment variables for containerd
type ProxyConfig struct {
	HTTPProxy  string `json:"httpProxy,omitempty"`
	HTTPSProxy string `json:"httpsProxy,omitempty"`
	NoProxy    string `json:"noProxy,omitempty"`
}

// Ports :
type Ports struct {
	HTTP  int32 `json:"http,omitempty"`
	HTTPS int32 `json:"https,omitempty"`
	TCP   int32 `json:"tcp,omitempty"`
}

// LoadBalancerConfig defines load-balancer configuration for the Cluster both for the control plane nodes and for the CPI
type LoadBalancerConfig struct {
	// UseOneArm defines the intent to une OneArm when upgrading CAPVCD from 0.5.x to 1.0.0
	UseOneArm bool   `json:"useOneArm,omitempty"`
	VipSubnet string `json:"vipSubnet,omitempty"`
}

// ZoneTopologyType defines the type of network topology used across zones in a Multi-AZ deployment
type ZoneTopologyType string

const (
	// DCGroup is the case where the networks in all zones are connected in a DC-Group to a single NSX-T instance and a
	// single Avi controller. All the OVDCs in the zones are connected to the same Edge Gateway. One Avi Controller
	// handles one Virtual Service that fronts the cluster.
	DCGroup ZoneTopologyType = "DCGroup"
	// UserSpecifiedEdgeGateway is a topology in which each OVDC has a separate NSX-T ALB and Avi controller. The routed
	// T-1 networking space is set up so that private IP addresses in one OVDC can route to private IP addresses in
	// another OVDC. From each Avi controller also, the private IPs are routable.
	//
	// For this topology, there is a designated zone that is chosen by the user. The edge on the user-specified zone is
	// used to create the Load-Balancer for the entire cluster.
	UserSpecifiedEdgeGateway ZoneTopologyType = "UserSpecifiedEdgeGateway"
	// ExternalLoadBalancer is the case where each zone has its own edge gateway. There needs to be a user-specified
	// load-balancer external to the cluster which is also managed by the user.
	//
	// On each edge gateway, there needs to be an LB configured. On each gateway, an LB to control-plane nodes in the
	// corresponding OVDC is created. There will be a separate layer that is pre-created and managed by the customer
	// which can multiplex across the load-balancers.
	ExternalLoadBalancer ZoneTopologyType = "ExternalLoadBalancer"
)

// DCGroupConfig defines configuration for DCGroup zone topology.
type DCGroupConfig struct {
	// TODO: decide on appropriate configuration for this type, or remove this struct
}

// UserSpecifiedEdgeGatewayConfig defines configuration for UserSpecifiedEdgeGateway zone topology.
type UserSpecifiedEdgeGatewayConfig struct {
	// EdgeGatewayZone defines the name of the user-provided zone containing an edge gateway.
	EdgeGatewayZone string `json:"edgeGatewayZone"`
}

// ExternalLoadBalancerConfig defines configuration for ExternalLoadBalancer zone topology.
type ExternalLoadBalancerConfig struct {
	// EdgeGatewayZones defines a list of zones containing an edge gateway.
	EdgeGatewayZones EdgeGatewayZones `json:"edgeGatewayZones"`
}

// EdgeGatewayZones defines a list of zones containing an edge gateway. For use with ExternalLoadBalancer topology.
type EdgeGatewayZones []EdgeGatewayZone

// EdgeGatewayZone provides the details of a zone containing an edge gateway. Used with ExternalLoadBalancer topology.
// topology.
type EdgeGatewayZone struct {
	// Name is the user-defined name of the zone representing the OVDC containing an edge gateway.
	Name string `json:"name"`
	// LoadBalancerIP is the IP address of the load balancer configured on the edge gateway for this zone.
	LoadBalancerIP string `json:"loadBalancerIP"`
	// LoadBalancerPort is the port of the load balancer configured on the edge gateway for this zone.
	LoadBalancerPort int `json:"loadBalancerPort"`
}

// Zone is an Availability Zone in VCD
type Zone struct {
	// Name defines the name of this zone.
	Name string `json:"name"`
	// OVDCName defines the actual name of the OVDC which corresponds to this zone.
	OVDCName string `json:"ovdcName"`
	// OVDCNetworkName defines the OVDC network for this zone.
	OVDCNetworkName string `json:"ovdcNetworkName"`
	// ControlPlaneZone defines whether a control plane node can be deployed to this zone.
	//+kubebuilder:default=false
	ControlPlaneZone bool `json:"controlPlaneZone"`
}

// MultiZoneSpec provides details of the zone configuration in the cluster as well as the NetworkTopology used. Only one
// of DCGroupConfig, UserSpecifiedEdgeGatewayConfig, or ExternalLoadBalancerConfig should be provided corresponding to the
// chosen ZoneTopology.
type MultiZoneSpec struct {
	// ZoneTopology defines the type of network topology used across zones in a Multi-AZ deployment.
	// Valid options are DCGroup, UserSpecifiedEdgeGateway, and ExternalLoadBalancer,
	// +optional
	ZoneTopology ZoneTopologyType `json:"zoneTopology,omitempty"`
	// DCGroupConfig contains configuration for DCGroup zone topology.
	// +optional
	DCGroupConfig DCGroupConfig `json:"dcGroupConfig,omitempty"`
	// UserSpecifiedEdgeGatewayConfig contains configuration for UserSpecifiedEdgeGateway zone topology.
	// +optional
	UserSpecifiedEdgeGatewayConfig UserSpecifiedEdgeGatewayConfig `json:"userSpecifiedEdgeGatewayConfig,omitempty"`
	// ExternalLoadBalancerConfig contains configuration for ExternalLoadBalancer zone topology.
	// +optional
	ExternalLoadBalancerConfig ExternalLoadBalancerConfig `json:"externalLoadBalancerConfig,omitempty"`
	// Zones defines the list of zones that this cluster should be deployed to.
	// +optional
	Zones []Zone `json:"zones,omitempty"`
}

// MultiZoneStatus provides the current status of the zone configuration in a Multi-AZ deployment.
type MultiZoneStatus struct {
	// ZoneTopology defines the type of network topology used across zones in a Multi-AZ deployment.
	// Valid options are DCGroup, UserSpecifiedEdgeGateway, and ExternalLoadBalancer
	// +optional
	ZoneTopology ZoneTopologyType `json:"zoneTopology,omitempty"`
	// DCGroupConfig contains configuration for DCGroup zone topology.
	// +optional
	DCGroupConfig DCGroupConfig `json:"dcGroupConfig,omitempty"`
	// UserSpecifiedEdgeGatewayConfig contains configuration for UserSpecifiedEdgeGateway zone topology.
	// +optional
	UserSpecifiedEdgeGatewayConfig UserSpecifiedEdgeGatewayConfig `json:"userSpecifiedEdgeGatewayConfig,omitempty"`
	// ExternalLoadBalancerConfig contains configuration for ExternalLoadBalancer zone topology.
	// +optional
	ExternalLoadBalancerConfig ExternalLoadBalancerConfig `json:"externalLoadBalancerConfig,omitempty"`
	// Zones defines the list of zones this cluster is configured with for a Mult-AZ deployment.
	// +optional
	Zones []Zone `json:"zones,omitempty"`
}

// VCDClusterSpec defines the desired state of VCDCluster
type VCDClusterSpec struct {
	// INSERT ADDITIONAL SPEC FIELDS - desired state of cluster
	// Important: Run "make" to regenerate code after modifying this file

	// +optional
	ControlPlaneEndpoint APIEndpoint `json:"controlPlaneEndpoint"`
	// +kubebuilder:validation:Required
	Site string `json:"site"`
	// +kubebuilder:validation:Required
	Org string `json:"org"`
	// +kubebuilder:validation:Required
	Ovdc string `json:"ovdc"`
	// +kubebuilder:validation:Required
	OvdcNetwork string `json:"ovdcNetwork"`
	// +kubebuilder:validation:Required
	UserCredentialsContext UserCredentialsContext `json:"userContext"`
	// + optional
	RDEId string `json:"rdeId,omitempty"`
	// +optional
	ParentUID string `json:"parentUid,omitempty"`
	// +optional
	//+kubebuilder:default=false
	UseAsManagementCluster bool `json:"useAsManagementCluster,omitempty"`
	// +optional
	ProxyConfigSpec ProxyConfig `json:"proxyConfigSpec,omitempty"`
	// +optional
	LoadBalancerConfigSpec LoadBalancerConfig `json:"loadBalancerConfigSpec,omitempty"`
	// MultiZoneSpec provides details of the configuration of the zones in the cluster as well as the NetworkTopologyType
	// used.
	// +optional
	MultiZoneSpec MultiZoneSpec `json:"multiZoneSpec,omitempty"`
	// OVDCZoneConfigMap defines the name of a config map storing the mapping Zone -> OVDC in a Multi-AZ
	// deployment. e.g. zone1 -> ovdc1, zone2 -> ovdc2
	// +optional
	OVDCZoneConfigMap string `json:"ovcdZoneConfigMap,omitempty"`
}

// VCDClusterStatus defines the observed state of VCDCluster
type VCDClusterStatus struct {
	// INSERT ADDITIONAL STATUS FIELD - define observed state of cluster
	// Important: Run "make" to regenerate code after modifying this file

	// Ready denotes that the vcd cluster (infrastructure) is ready.
	// +kubebuilder:default=false
	Ready bool `json:"ready"`

	// RdeVersionInUse indicates the version of capvcdCluster entity type used by CAPVCD.
	// +kubebuilder:default="1.1.0"
	RdeVersionInUse string `json:"rdeVersionInUse"`

	// MetadataUpdated denotes that the metadata of Vapp is updated.
	// +optional
	VAppMetadataUpdated bool `json:"vappMetadataUpdated,omitempty"`

	// Conditions defines current service state of the VCDCluster.
	// +optional
	Conditions clusterv1.Conditions `json:"conditions,omitempty"`

	// optional
	Site string `json:"site,omitempty"`

	// optional
	VcdResourceMap VCDResourceMap `json:"vcdResourceMap,omitempty"`

	// optional
	Org string `json:"org,omitempty"`

	// optional
	Ovdc string `json:"ovdc,omitempty"`

	// optional
	OvdcNetwork string `json:"ovdcNetwork,omitempty"`

	// +optional
	InfraId string `json:"infraId,omitempty"`

	// +optional
	ParentUID string `json:"parentUid,omitempty"`

	// +optional
	UseAsManagementCluster bool `json:"useAsManagementCluster,omitempty"`

	// +optional
	ProxyConfig ProxyConfig `json:"proxyConfig,omitempty"`

	// +optional
	LoadBalancerConfig LoadBalancerConfig `json:"loadBalancerConfig,omitempty"`

	// FailureDomains lists the zones of this cluster. This field is parsed from the Zones field of
	// vcdCluster.MultiZoneSpec if set up appropriately.
	// +optional
	FailureDomains clusterv1.FailureDomains `json:"failureDomains,omitempty"`

	// MultiZoneStatus provides the current status of the multi-zone configuration in a Multi-AZ deployment
	// +optional
	MultiZoneStatus MultiZoneStatus `json:"multiZoneStatus,omitempty"`
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:storageversion
// VCDCluster is the Schema for the vcdclusters API
type VCDCluster struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	// +kubebuilder:validation:Required
	Spec   VCDClusterSpec   `json:"spec"`
	Status VCDClusterStatus `json:"status,omitempty"`
}

//+kubebuilder:object:root=true

// VCDClusterList contains a list of VCDCluster
type VCDClusterList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []VCDCluster `json:"items"`
}

// GetConditions returns the set of conditions for this object.
func (c *VCDCluster) GetConditions() clusterv1.Conditions {
	return c.Status.Conditions
}

// SetConditions sets the conditions on this object.
func (c *VCDCluster) SetConditions(conditions clusterv1.Conditions) {
	c.Status.Conditions = conditions
}
func init() {
	SchemeBuilder.Register(&VCDCluster{}, &VCDClusterList{})
}
