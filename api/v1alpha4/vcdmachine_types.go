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
	// MachineFinalizer allows ReconcileDockerMachine to clean up resources associated with AWSMachine before
	// removing it from the apiserver.
	MachineFinalizer = "vcdmachine.infrastructure.cluster.x-k8s.io"
	VCDProviderID    = "vmware-cloud-director"
)

// VCDMachineSpec defines the desired state of VCDMachine
type VCDMachineSpec struct {
	// INSERT ADDITIONAL SPEC FIELDS - desired state of cluster
	// Important: Run "make" to regenerate code after modifying this file

	// ProviderID will be the container name in ProviderID format (vmware-cloud-director://<vm id>)
	// +optional
	ProviderID *string `json:"providerID,omitempty"`

	// Catalog hosting templates
	// +optional
	Catalog string `json:"catalog,omitempty"`

	// TemplatePath is the path of the template OVA that is to be used
	// +optional
	Template string `json:"template,omitempty"`

	// ComputePolicy is the compute policy to be used on this machine
	// +optional
	ComputePolicy string `json:"computePolicy,omitempty"`

	// Bootstrapped is true when the kubeadm bootstrapping has been run
	// against this machine
	// +optional
	Bootstrapped bool `json:"bootstrapped,omitempty"`
}

// VCDMachineStatus defines the observed state of VCDMachine
type VCDMachineStatus struct {
	// INSERT ADDITIONAL STATUS FIELD - define observed state of cluster
	// Important: Run "make" to regenerate code after modifying this file

	// Ready denotes that the machine (docker container) is ready
	// +optional
	Ready bool `json:"ready"`

	// Addresses contains the associated addresses for the docker machine.
	// +optional
	Addresses []clusterv1.MachineAddress `json:"addresses,omitempty"`

	// Conditions defines current service state of the DockerMachine.
	// +optional
	Conditions clusterv1.Conditions `json:"conditions,omitempty"`
}

//+kubebuilder:object:root=true
//+kubebuilder:subresource:status

// VCDMachine is the Schema for the vcdmachines API
type VCDMachine struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   VCDMachineSpec   `json:"spec,omitempty"`
	Status VCDMachineStatus `json:"status,omitempty"`
}

//+kubebuilder:object:root=true

// VCDMachineList contains a list of VCDMachine
type VCDMachineList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []VCDMachine `json:"items"`
}

// GetConditions returns the set of conditions for this object.
func (c *VCDMachine) GetConditions() clusterv1.Conditions {
	return c.Status.Conditions
}

// SetConditions sets the conditions on this object.
func (c *VCDMachine) SetConditions(conditions clusterv1.Conditions) {
	c.Status.Conditions = conditions
}
func init() {
	SchemeBuilder.Register(&VCDMachine{}, &VCDMachineList{})
}
