/*
   Copyright 2021 VMware, Inc.
   SPDX-License-Identifier: Apache-2.0
*/

package v1alpha4

import (
	"github.com/vmware/cluster-api-provider-cloud-director/api/v1beta3"
	"k8s.io/apimachinery/pkg/conversion"
)

func Convert_v1beta3_VCDClusterSpec_To_v1alpha4_VCDClusterSpec(in *v1beta3.VCDClusterSpec, out *VCDClusterSpec, s conversion.Scope) error {
	return autoConvert_v1beta3_VCDClusterSpec_To_v1alpha4_VCDClusterSpec(in, out, s)
}

func Convert_v1alpha4_VCDMachineSpec_To_v1beta3_VCDMachineSpec(in *VCDMachineSpec, out *v1beta3.VCDMachineSpec, s conversion.Scope) error {
	return autoConvert_v1alpha4_VCDMachineSpec_To_v1beta3_VCDMachineSpec(in, out, s)
}

func Convert_v1beta3_VCDMachineSpec_To_v1alpha4_VCDMachineSpec(in *v1beta3.VCDMachineSpec, out *VCDMachineSpec, s conversion.Scope) error {
	return autoConvert_v1beta3_VCDMachineSpec_To_v1alpha4_VCDMachineSpec(in, out, s)
}
func Convert_v1beta3_VCDClusterStatus_To_v1alpha4_VCDClusterStatus(in *v1beta3.VCDClusterStatus, out *VCDClusterStatus, s conversion.Scope) error {
	// Todo: check if VCDClusterStatus.vAppMetadata_Updated needs to be updated (VCDA-3532)
	return autoConvert_v1beta3_VCDClusterStatus_To_v1alpha4_VCDClusterStatus(in, out, s)
}

func Convert_v1alpha4_VCDClusterSpec_To_v1beta3_VCDClusterSpec(in *VCDClusterSpec, out *v1beta3.VCDClusterSpec, s conversion.Scope) error {
	return autoConvert_v1alpha4_VCDClusterSpec_To_v1beta3_VCDClusterSpec(in, out, s)
}

func Convert_v1beta3_VCDMachineStatus_To_v1alpha4_VCDMachineStatus(in *v1beta3.VCDMachineStatus, out *VCDMachineStatus, s conversion.Scope) error {
	return autoConvert_v1beta3_VCDMachineStatus_To_v1alpha4_VCDMachineStatus(in, out, s)
}
func Convert_v1beta3_UserCredentialsContext_To_v1alpha4_UserCredentialsContext(in *v1beta3.UserCredentialsContext, out *UserCredentialsContext, s conversion.Scope) error {
	return autoConvert_v1beta3_UserCredentialsContext_To_v1alpha4_UserCredentialsContext(in, out, s)
}

func Convert_v1beta3_VCDMachineTemplateResource_To_v1alpha4_VCDMachineTemplateResource(in *v1beta3.VCDMachineTemplateResource, out *VCDMachineTemplateResource, s conversion.Scope) error {
	return autoConvert_v1beta3_VCDMachineTemplateResource_To_v1alpha4_VCDMachineTemplateResource(in, out, s)
}
