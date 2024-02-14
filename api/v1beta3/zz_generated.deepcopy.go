//go:build !ignore_autogenerated

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

// Code generated by controller-gen. DO NOT EDIT.

package v1beta3

import (
	"k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/cluster-api/api/v1beta1"
)

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *APIEndpoint) DeepCopyInto(out *APIEndpoint) {
	*out = *in
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new APIEndpoint.
func (in *APIEndpoint) DeepCopy() *APIEndpoint {
	if in == nil {
		return nil
	}
	out := new(APIEndpoint)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *DCGroupConfig) DeepCopyInto(out *DCGroupConfig) {
	*out = *in
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new DCGroupConfig.
func (in *DCGroupConfig) DeepCopy() *DCGroupConfig {
	if in == nil {
		return nil
	}
	out := new(DCGroupConfig)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *EdgeGatewayZone) DeepCopyInto(out *EdgeGatewayZone) {
	*out = *in
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new EdgeGatewayZone.
func (in *EdgeGatewayZone) DeepCopy() *EdgeGatewayZone {
	if in == nil {
		return nil
	}
	out := new(EdgeGatewayZone)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in EdgeGatewayZones) DeepCopyInto(out *EdgeGatewayZones) {
	{
		in := &in
		*out = make(EdgeGatewayZones, len(*in))
		copy(*out, *in)
	}
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new EdgeGatewayZones.
func (in EdgeGatewayZones) DeepCopy() EdgeGatewayZones {
	if in == nil {
		return nil
	}
	out := new(EdgeGatewayZones)
	in.DeepCopyInto(out)
	return *out
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *ExternalLoadBalancerConfig) DeepCopyInto(out *ExternalLoadBalancerConfig) {
	*out = *in
	if in.EdgeGatewayZones != nil {
		in, out := &in.EdgeGatewayZones, &out.EdgeGatewayZones
		*out = make(EdgeGatewayZones, len(*in))
		copy(*out, *in)
	}
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new ExternalLoadBalancerConfig.
func (in *ExternalLoadBalancerConfig) DeepCopy() *ExternalLoadBalancerConfig {
	if in == nil {
		return nil
	}
	out := new(ExternalLoadBalancerConfig)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *LoadBalancerConfig) DeepCopyInto(out *LoadBalancerConfig) {
	*out = *in
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new LoadBalancerConfig.
func (in *LoadBalancerConfig) DeepCopy() *LoadBalancerConfig {
	if in == nil {
		return nil
	}
	out := new(LoadBalancerConfig)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *MultiZoneSpec) DeepCopyInto(out *MultiZoneSpec) {
	*out = *in
	out.DCGroupConfig = in.DCGroupConfig
	out.UserSpecifiedEdgeGatewayConfig = in.UserSpecifiedEdgeGatewayConfig
	in.ExternalLoadBalancerConfig.DeepCopyInto(&out.ExternalLoadBalancerConfig)
	if in.Zones != nil {
		in, out := &in.Zones, &out.Zones
		*out = make([]Zone, len(*in))
		copy(*out, *in)
	}
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new MultiZoneSpec.
func (in *MultiZoneSpec) DeepCopy() *MultiZoneSpec {
	if in == nil {
		return nil
	}
	out := new(MultiZoneSpec)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *MultiZoneStatus) DeepCopyInto(out *MultiZoneStatus) {
	*out = *in
	out.DCGroupConfig = in.DCGroupConfig
	out.UserSpecifiedEdgeGatewayConfig = in.UserSpecifiedEdgeGatewayConfig
	in.ExternalLoadBalancerConfig.DeepCopyInto(&out.ExternalLoadBalancerConfig)
	if in.Zones != nil {
		in, out := &in.Zones, &out.Zones
		*out = make([]Zone, len(*in))
		copy(*out, *in)
	}
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new MultiZoneStatus.
func (in *MultiZoneStatus) DeepCopy() *MultiZoneStatus {
	if in == nil {
		return nil
	}
	out := new(MultiZoneStatus)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *Ports) DeepCopyInto(out *Ports) {
	*out = *in
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new Ports.
func (in *Ports) DeepCopy() *Ports {
	if in == nil {
		return nil
	}
	out := new(Ports)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *ProxyConfig) DeepCopyInto(out *ProxyConfig) {
	*out = *in
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new ProxyConfig.
func (in *ProxyConfig) DeepCopy() *ProxyConfig {
	if in == nil {
		return nil
	}
	out := new(ProxyConfig)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *UserCredentialsContext) DeepCopyInto(out *UserCredentialsContext) {
	*out = *in
	if in.SecretRef != nil {
		in, out := &in.SecretRef, &out.SecretRef
		*out = new(v1.SecretReference)
		**out = **in
	}
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new UserCredentialsContext.
func (in *UserCredentialsContext) DeepCopy() *UserCredentialsContext {
	if in == nil {
		return nil
	}
	out := new(UserCredentialsContext)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *UserSpecifiedEdgeGatewayConfig) DeepCopyInto(out *UserSpecifiedEdgeGatewayConfig) {
	*out = *in
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new UserSpecifiedEdgeGatewayConfig.
func (in *UserSpecifiedEdgeGatewayConfig) DeepCopy() *UserSpecifiedEdgeGatewayConfig {
	if in == nil {
		return nil
	}
	out := new(UserSpecifiedEdgeGatewayConfig)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *VCDCluster) DeepCopyInto(out *VCDCluster) {
	*out = *in
	out.TypeMeta = in.TypeMeta
	in.ObjectMeta.DeepCopyInto(&out.ObjectMeta)
	in.Spec.DeepCopyInto(&out.Spec)
	in.Status.DeepCopyInto(&out.Status)
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new VCDCluster.
func (in *VCDCluster) DeepCopy() *VCDCluster {
	if in == nil {
		return nil
	}
	out := new(VCDCluster)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyObject is an autogenerated deepcopy function, copying the receiver, creating a new runtime.Object.
func (in *VCDCluster) DeepCopyObject() runtime.Object {
	if c := in.DeepCopy(); c != nil {
		return c
	}
	return nil
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *VCDClusterList) DeepCopyInto(out *VCDClusterList) {
	*out = *in
	out.TypeMeta = in.TypeMeta
	in.ListMeta.DeepCopyInto(&out.ListMeta)
	if in.Items != nil {
		in, out := &in.Items, &out.Items
		*out = make([]VCDCluster, len(*in))
		for i := range *in {
			(*in)[i].DeepCopyInto(&(*out)[i])
		}
	}
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new VCDClusterList.
func (in *VCDClusterList) DeepCopy() *VCDClusterList {
	if in == nil {
		return nil
	}
	out := new(VCDClusterList)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyObject is an autogenerated deepcopy function, copying the receiver, creating a new runtime.Object.
func (in *VCDClusterList) DeepCopyObject() runtime.Object {
	if c := in.DeepCopy(); c != nil {
		return c
	}
	return nil
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *VCDClusterSpec) DeepCopyInto(out *VCDClusterSpec) {
	*out = *in
	out.ControlPlaneEndpoint = in.ControlPlaneEndpoint
	in.UserCredentialsContext.DeepCopyInto(&out.UserCredentialsContext)
	out.ProxyConfigSpec = in.ProxyConfigSpec
	out.LoadBalancerConfigSpec = in.LoadBalancerConfigSpec
	in.MultiZoneSpec.DeepCopyInto(&out.MultiZoneSpec)
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new VCDClusterSpec.
func (in *VCDClusterSpec) DeepCopy() *VCDClusterSpec {
	if in == nil {
		return nil
	}
	out := new(VCDClusterSpec)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *VCDClusterStatus) DeepCopyInto(out *VCDClusterStatus) {
	*out = *in
	if in.Conditions != nil {
		in, out := &in.Conditions, &out.Conditions
		*out = make(v1beta1.Conditions, len(*in))
		for i := range *in {
			(*in)[i].DeepCopyInto(&(*out)[i])
		}
	}
	in.VcdResourceMap.DeepCopyInto(&out.VcdResourceMap)
	out.ProxyConfig = in.ProxyConfig
	out.LoadBalancerConfig = in.LoadBalancerConfig
	if in.FailureDomains != nil {
		in, out := &in.FailureDomains, &out.FailureDomains
		*out = make(v1beta1.FailureDomains, len(*in))
		for key, val := range *in {
			(*out)[key] = *val.DeepCopy()
		}
	}
	in.MultiZoneStatus.DeepCopyInto(&out.MultiZoneStatus)
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new VCDClusterStatus.
func (in *VCDClusterStatus) DeepCopy() *VCDClusterStatus {
	if in == nil {
		return nil
	}
	out := new(VCDClusterStatus)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *VCDClusterTemplate) DeepCopyInto(out *VCDClusterTemplate) {
	*out = *in
	out.TypeMeta = in.TypeMeta
	in.ObjectMeta.DeepCopyInto(&out.ObjectMeta)
	in.Spec.DeepCopyInto(&out.Spec)
	out.Status = in.Status
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new VCDClusterTemplate.
func (in *VCDClusterTemplate) DeepCopy() *VCDClusterTemplate {
	if in == nil {
		return nil
	}
	out := new(VCDClusterTemplate)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyObject is an autogenerated deepcopy function, copying the receiver, creating a new runtime.Object.
func (in *VCDClusterTemplate) DeepCopyObject() runtime.Object {
	if c := in.DeepCopy(); c != nil {
		return c
	}
	return nil
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *VCDClusterTemplateList) DeepCopyInto(out *VCDClusterTemplateList) {
	*out = *in
	out.TypeMeta = in.TypeMeta
	in.ListMeta.DeepCopyInto(&out.ListMeta)
	if in.Items != nil {
		in, out := &in.Items, &out.Items
		*out = make([]VCDClusterTemplate, len(*in))
		for i := range *in {
			(*in)[i].DeepCopyInto(&(*out)[i])
		}
	}
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new VCDClusterTemplateList.
func (in *VCDClusterTemplateList) DeepCopy() *VCDClusterTemplateList {
	if in == nil {
		return nil
	}
	out := new(VCDClusterTemplateList)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyObject is an autogenerated deepcopy function, copying the receiver, creating a new runtime.Object.
func (in *VCDClusterTemplateList) DeepCopyObject() runtime.Object {
	if c := in.DeepCopy(); c != nil {
		return c
	}
	return nil
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *VCDClusterTemplateResource) DeepCopyInto(out *VCDClusterTemplateResource) {
	*out = *in
	in.Spec.DeepCopyInto(&out.Spec)
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new VCDClusterTemplateResource.
func (in *VCDClusterTemplateResource) DeepCopy() *VCDClusterTemplateResource {
	if in == nil {
		return nil
	}
	out := new(VCDClusterTemplateResource)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *VCDClusterTemplateSpec) DeepCopyInto(out *VCDClusterTemplateSpec) {
	*out = *in
	in.Template.DeepCopyInto(&out.Template)
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new VCDClusterTemplateSpec.
func (in *VCDClusterTemplateSpec) DeepCopy() *VCDClusterTemplateSpec {
	if in == nil {
		return nil
	}
	out := new(VCDClusterTemplateSpec)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *VCDClusterTemplateStatus) DeepCopyInto(out *VCDClusterTemplateStatus) {
	*out = *in
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new VCDClusterTemplateStatus.
func (in *VCDClusterTemplateStatus) DeepCopy() *VCDClusterTemplateStatus {
	if in == nil {
		return nil
	}
	out := new(VCDClusterTemplateStatus)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *VCDMachine) DeepCopyInto(out *VCDMachine) {
	*out = *in
	out.TypeMeta = in.TypeMeta
	in.ObjectMeta.DeepCopyInto(&out.ObjectMeta)
	in.Spec.DeepCopyInto(&out.Spec)
	in.Status.DeepCopyInto(&out.Status)
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new VCDMachine.
func (in *VCDMachine) DeepCopy() *VCDMachine {
	if in == nil {
		return nil
	}
	out := new(VCDMachine)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyObject is an autogenerated deepcopy function, copying the receiver, creating a new runtime.Object.
func (in *VCDMachine) DeepCopyObject() runtime.Object {
	if c := in.DeepCopy(); c != nil {
		return c
	}
	return nil
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *VCDMachineList) DeepCopyInto(out *VCDMachineList) {
	*out = *in
	out.TypeMeta = in.TypeMeta
	in.ListMeta.DeepCopyInto(&out.ListMeta)
	if in.Items != nil {
		in, out := &in.Items, &out.Items
		*out = make([]VCDMachine, len(*in))
		for i := range *in {
			(*in)[i].DeepCopyInto(&(*out)[i])
		}
	}
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new VCDMachineList.
func (in *VCDMachineList) DeepCopy() *VCDMachineList {
	if in == nil {
		return nil
	}
	out := new(VCDMachineList)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyObject is an autogenerated deepcopy function, copying the receiver, creating a new runtime.Object.
func (in *VCDMachineList) DeepCopyObject() runtime.Object {
	if c := in.DeepCopy(); c != nil {
		return c
	}
	return nil
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *VCDMachineSpec) DeepCopyInto(out *VCDMachineSpec) {
	*out = *in
	if in.ProviderID != nil {
		in, out := &in.ProviderID, &out.ProviderID
		*out = new(string)
		**out = **in
	}
	out.DiskSize = in.DiskSize.DeepCopy()
	if in.ExtraOvdcNetworks != nil {
		in, out := &in.ExtraOvdcNetworks, &out.ExtraOvdcNetworks
		*out = make([]string, len(*in))
		copy(*out, *in)
	}
	if in.FailureDomain != nil {
		in, out := &in.FailureDomain, &out.FailureDomain
		*out = new(string)
		**out = **in
	}
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new VCDMachineSpec.
func (in *VCDMachineSpec) DeepCopy() *VCDMachineSpec {
	if in == nil {
		return nil
	}
	out := new(VCDMachineSpec)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *VCDMachineStatus) DeepCopyInto(out *VCDMachineStatus) {
	*out = *in
	if in.ProviderID != nil {
		in, out := &in.ProviderID, &out.ProviderID
		*out = new(string)
		**out = **in
	}
	if in.Addresses != nil {
		in, out := &in.Addresses, &out.Addresses
		*out = make([]v1beta1.MachineAddress, len(*in))
		copy(*out, *in)
	}
	out.DiskSize = in.DiskSize.DeepCopy()
	if in.Conditions != nil {
		in, out := &in.Conditions, &out.Conditions
		*out = make(v1beta1.Conditions, len(*in))
		for i := range *in {
			(*in)[i].DeepCopyInto(&(*out)[i])
		}
	}
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new VCDMachineStatus.
func (in *VCDMachineStatus) DeepCopy() *VCDMachineStatus {
	if in == nil {
		return nil
	}
	out := new(VCDMachineStatus)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *VCDMachineTemplate) DeepCopyInto(out *VCDMachineTemplate) {
	*out = *in
	out.TypeMeta = in.TypeMeta
	in.ObjectMeta.DeepCopyInto(&out.ObjectMeta)
	in.Spec.DeepCopyInto(&out.Spec)
	out.Status = in.Status
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new VCDMachineTemplate.
func (in *VCDMachineTemplate) DeepCopy() *VCDMachineTemplate {
	if in == nil {
		return nil
	}
	out := new(VCDMachineTemplate)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyObject is an autogenerated deepcopy function, copying the receiver, creating a new runtime.Object.
func (in *VCDMachineTemplate) DeepCopyObject() runtime.Object {
	if c := in.DeepCopy(); c != nil {
		return c
	}
	return nil
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *VCDMachineTemplateList) DeepCopyInto(out *VCDMachineTemplateList) {
	*out = *in
	out.TypeMeta = in.TypeMeta
	in.ListMeta.DeepCopyInto(&out.ListMeta)
	if in.Items != nil {
		in, out := &in.Items, &out.Items
		*out = make([]VCDMachineTemplate, len(*in))
		for i := range *in {
			(*in)[i].DeepCopyInto(&(*out)[i])
		}
	}
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new VCDMachineTemplateList.
func (in *VCDMachineTemplateList) DeepCopy() *VCDMachineTemplateList {
	if in == nil {
		return nil
	}
	out := new(VCDMachineTemplateList)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyObject is an autogenerated deepcopy function, copying the receiver, creating a new runtime.Object.
func (in *VCDMachineTemplateList) DeepCopyObject() runtime.Object {
	if c := in.DeepCopy(); c != nil {
		return c
	}
	return nil
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *VCDMachineTemplateResource) DeepCopyInto(out *VCDMachineTemplateResource) {
	*out = *in
	in.ObjectMeta.DeepCopyInto(&out.ObjectMeta)
	in.Spec.DeepCopyInto(&out.Spec)
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new VCDMachineTemplateResource.
func (in *VCDMachineTemplateResource) DeepCopy() *VCDMachineTemplateResource {
	if in == nil {
		return nil
	}
	out := new(VCDMachineTemplateResource)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *VCDMachineTemplateSpec) DeepCopyInto(out *VCDMachineTemplateSpec) {
	*out = *in
	in.Template.DeepCopyInto(&out.Template)
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new VCDMachineTemplateSpec.
func (in *VCDMachineTemplateSpec) DeepCopy() *VCDMachineTemplateSpec {
	if in == nil {
		return nil
	}
	out := new(VCDMachineTemplateSpec)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *VCDMachineTemplateStatus) DeepCopyInto(out *VCDMachineTemplateStatus) {
	*out = *in
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new VCDMachineTemplateStatus.
func (in *VCDMachineTemplateStatus) DeepCopy() *VCDMachineTemplateStatus {
	if in == nil {
		return nil
	}
	out := new(VCDMachineTemplateStatus)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *VCDResource) DeepCopyInto(out *VCDResource) {
	*out = *in
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new VCDResource.
func (in *VCDResource) DeepCopy() *VCDResource {
	if in == nil {
		return nil
	}
	out := new(VCDResource)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *VCDResourceMap) DeepCopyInto(out *VCDResourceMap) {
	*out = *in
	if in.Ovdcs != nil {
		in, out := &in.Ovdcs, &out.Ovdcs
		*out = make(VCDResources, len(*in))
		copy(*out, *in)
	}
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new VCDResourceMap.
func (in *VCDResourceMap) DeepCopy() *VCDResourceMap {
	if in == nil {
		return nil
	}
	out := new(VCDResourceMap)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in VCDResources) DeepCopyInto(out *VCDResources) {
	{
		in := &in
		*out = make(VCDResources, len(*in))
		copy(*out, *in)
	}
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new VCDResources.
func (in VCDResources) DeepCopy() VCDResources {
	if in == nil {
		return nil
	}
	out := new(VCDResources)
	in.DeepCopyInto(out)
	return *out
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *Zone) DeepCopyInto(out *Zone) {
	*out = *in
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new Zone.
func (in *Zone) DeepCopy() *Zone {
	if in == nil {
		return nil
	}
	out := new(Zone)
	in.DeepCopyInto(out)
	return out
}
