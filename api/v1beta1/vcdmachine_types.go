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
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	clusterv1 "sigs.k8s.io/cluster-api/api/v1beta1"
)

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

	// SizingPolicy is the sizing policy to be used on this machine.
	// If no sizing policy is specified, default sizing policy will be used to create the nodes
	// +optional
	SizingPolicy string `json:"sizingPolicy,omitempty"`

	// PlacementPolicy is the placement policy to be used on this machine.
	// +optional
	PlacementPolicy string `json:"placementPolicy,omitempty"`

	// StorageProfile is the storage profile to be used on this machine
	// +optional
	StorageProfile string `json:"storageProfile,omitempty"`

	// DiskSize is the size, in bytes, of the disk for this machine
	// +optional
	DiskSize resource.Quantity `json:"diskSize,omitempty"`

	// Bootstrapped is true when the kubeadm bootstrapping has been run
	// against this machine
	// +optional
	Bootstrapped bool `json:"bootstrapped,omitempty"`

	// NvidiaGPU is true when a VM should be created with the relevant binaries installed
	// If true, then an appropriate placement policy should be set
	// +optional
	NvidiaGPU bool `json:"nvidiaGPU,omitempty"`
}

// VCDMachineStatus defines the observed state of VCDMachine
type VCDMachineStatus struct {
	// INSERT ADDITIONAL STATUS FIELD - define observed state of cluster
	// Important: Run "make" to regenerate code after modifying this file

	// ProviderID will be the container name in ProviderID format (vmware-cloud-director://<vm id>)
	// +optional
	ProviderID *string `json:"providerID,omitempty"`

	// Ready denotes that the machine (docker container) is ready
	// +optional
	Ready bool `json:"ready"`

	// Addresses contains the associated addresses for the docker machine.
	// +optional
	Addresses []clusterv1.MachineAddress `json:"addresses,omitempty"`

	// Template is the path of the template OVA that is to be used
	// +optional
	Template string `json:"template,omitempty"`

	// Conditions defines current service state of the DockerMachine.
	// +optional
	Conditions clusterv1.Conditions `json:"conditions,omitempty"`
}

//+kubebuilder:object:root=true
//+kubebuilder:subresource:status
//+kubebuilder:storageversion

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
