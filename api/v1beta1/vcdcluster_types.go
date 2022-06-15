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

package v1beta1

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

type DefaultStorageClassOptions struct {
	VCDStorageProfileName string `json:"vcdStorageProfileName"`
	// +kubebuilder:default=cloud-director-default
	K8sStorageClassName string `json:"k8sStorageClassName,omitempty"`
	// +kubebuilder:default=false
	UseDeleteReclaimPolicy bool `json:"useDeleteReclaimPolicy,omitempty"`
	// +kubebuilder:default=ext4
	FileSystem string `json:"fileSystem,omitempty"`
}

// ProxyConfig defines HTTP proxy environment variables for containerd
type ProxyConfig struct {
	HTTPProxy  string `json:"httpProxy,omitempty"`
	HTTPSProxy string `json:"httpsProxy,omitempty"`
	NoProxy    string `json:"noProxy,omitempty"`
}

// LoadBalancer defines load-balancer configuration for the Cluster both for the control plane nodes and for the CPI
type LoadBalancer struct {
	UseOneArm bool `json:"useOneArm,omitempty"`
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
	// +optional
	DefaultStorageClassOptions DefaultStorageClassOptions `json:"defaultStorageClassOptions"`
	// + optional
	RDEId string `json:"rdeId,omitempty"`
	// +optional
	ParentUID string `json:"parentUid,omitempty"`
	// +optional
	UseAsManagementCluster bool `json:"useAsManagementCluster,omitempty"`
	// +optional
	ProxyConfig ProxyConfig `json:"proxyConfig,omitempty"`
	// +optional
	LoadBalancer LoadBalancer `json:"loadBalancer,omitempty"`
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
	VAppMetadataUpdated bool `json:"vappmetadataUpdated,omitempty"`

	// Conditions defines current service state of the VCDCluster.
	// +optional
	Conditions clusterv1.Conditions `json:"conditions,omitempty"`

	// +optional
	InfraId string `json:"infraId,omitempty"`

	// +optional
	ParentUID string `json:"parentUid,omitempty"`

	// +optional
	UseAsManagementCluster bool `json:"useAsManagementCluster,omitempty"`

	// +optional
	ProxyConfig ProxyConfig `json:"proxyConfig,omitempty"`
}

//+kubebuilder:object:root=true
//+kubebuilder:subresource:status
//+kubebuilder:storageversion

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
