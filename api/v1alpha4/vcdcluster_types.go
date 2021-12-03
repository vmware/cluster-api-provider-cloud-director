/*
   Copyright 2021 VMware, Inc.
   SPDX-License-Identifier: Apache-2.0
*/

package v1alpha4

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	clusterv1 "sigs.k8s.io/cluster-api/api/v1alpha4"
)

// EDIT THIS FILE!  THIS IS SCAFFOLDING FOR YOU TO OWN!
// NOTE: json tags are required.  Any new fields you add must have json tags for the fields to be serialized.

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
	Username     string `json:"username,omitempty"`
	Password     string `json:"password,omitempty"`
	RefreshToken string `json:"refreshToken,omitempty"`
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
	DefaultComputePolicy string `json:"defaultComputePolicy,omitempty"`
}

// VCDClusterStatus defines the observed state of VCDCluster
type VCDClusterStatus struct {
	// INSERT ADDITIONAL STATUS FIELD - define observed state of cluster
	// Important: Run "make" to regenerate code after modifying this file

	// Ready denotes that the vcd cluster (infrastructure) is ready.
	Ready bool `json:"ready"`

	// Conditions defines current service state of the VCDCluster.
	// +optional
	Conditions clusterv1.Conditions `json:"conditions,omitempty"`
	// +optional
	ClusterRDEId string `json:"clusterRDEId,omitempty"`
}

//+kubebuilder:object:root=true
//+kubebuilder:subresource:status

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
