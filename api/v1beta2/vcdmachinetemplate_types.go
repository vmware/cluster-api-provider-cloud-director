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

package v1beta2

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

type VCDMachineTemplateResource struct {
	// Spec is the specification of the desired behavior of the machine.
	Spec VCDMachineSpec `json:"spec"`
}

// VCDMachineTemplateSpec defines the desired state of VCDMachineTemplate
type VCDMachineTemplateSpec struct {
	// INSERT ADDITIONAL SPEC FIELDS - desired state of cluster
	// Important: Run "make" to regenerate code after modifying this file

	Template VCDMachineTemplateResource `json:"template"`
}

// VCDMachineTemplateStatus defines the observed state of VCDMachineTemplate
type VCDMachineTemplateStatus struct {
	// INSERT ADDITIONAL STATUS FIELD - define observed state of cluster
	// Important: Run "make" to regenerate code after modifying this file
}

//+kubebuilder:object:root=true
//+kubebuilder:subresource:status
//+kubebuilder:storageversion

// VCDMachineTemplate is the Schema for the vcdmachinetemplates API
type VCDMachineTemplate struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   VCDMachineTemplateSpec   `json:"spec,omitempty"`
	Status VCDMachineTemplateStatus `json:"status,omitempty"`
}

//+kubebuilder:object:root=true

// VCDMachineTemplateList contains a list of VCDMachineTemplate
type VCDMachineTemplateList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []VCDMachineTemplate `json:"items"`
}

func init() {
	SchemeBuilder.Register(&VCDMachineTemplate{}, &VCDMachineTemplateList{})
}
