/*
Copyright 2022.

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

package v1alpha1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// EDIT THIS FILE!  THIS IS SCAFFOLDING FOR YOU TO OWN!
// NOTE: json tags are required.  Any new fields you add must have json tags for the fields to be serialized.

type UserCredentialsContext struct {
	Username     string `json:"username,omitempty"`
	Password     string `json:"password,omitempty"`
	RefreshToken string `json:"refreshToken,omitempty"`
}

// RDEProjectorSpec defines the desired state of RDEProjector
type RDEProjectorSpec struct {
	// INSERT ADDITIONAL SPEC FIELDS - desired state of cluster
	// Important: Run "make" to regenerate code after modifying this file

	// +kubebuilder:validation:Required
	RDEId string `json:"rdeId,omitempty"`
	// +kubebuilder:validation:Required
	Site string `json:"site"`
	// +kubebuilder:validation:Required
	Org string `json:"org"`
	// +kubebuilder:validation:Required
	UserCredentialsContext UserCredentialsContext `json:"userContext"`
}

// RDEProjectorStatus defines the observed state of RDEProjector
type RDEProjectorStatus struct {
	// TODO Add necessary fields from Spec
}

//+kubebuilder:object:root=true
//+kubebuilder:subresource:status

// RDEProjector is the Schema for the rdeprojectors API
type RDEProjector struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   RDEProjectorSpec   `json:"spec,omitempty"`
	Status RDEProjectorStatus `json:"status,omitempty"`
}

//+kubebuilder:object:root=true

// RDEProjectorList contains a list of RDEProjector
type RDEProjectorList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []RDEProjector `json:"items"`
}

func init() {
	SchemeBuilder.Register(&RDEProjector{}, &RDEProjectorList{})
}
