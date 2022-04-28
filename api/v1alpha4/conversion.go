/*
   Copyright 2021 VMware, Inc.
   SPDX-License-Identifier: Apache-2.0
*/

package v1alpha4

import (
	"github.com/vmware/cluster-api-provider-cloud-director/api/v1beta1"
	"k8s.io/apimachinery/pkg/conversion"
)

func Convert_v1beta1_VCDClusterSpec_To_v1alpha4_VCDClusterSpec(in *v1beta1.VCDClusterSpec, out *VCDClusterSpec, s conversion.Scope) error {
	return autoConvert_v1beta1_VCDClusterSpec_To_v1alpha4_VCDClusterSpec(in, out, s)
}

func Convert_v1alpha4_VCDMachineSpec_To_v1beta1_VCDMachineSpec(in *VCDMachineSpec, out *v1beta1.VCDMachineSpec, s conversion.Scope) error {
	return autoConvert_v1alpha4_VCDMachineSpec_To_v1beta1_VCDMachineSpec(in, out, s)
}

func Convert_v1beta1_VCDMachineSpec_To_v1alpha4_VCDMachineSpec(in *v1beta1.VCDMachineSpec, out *VCDMachineSpec, s conversion.Scope) error {
	return autoConvert_v1beta1_VCDMachineSpec_To_v1alpha4_VCDMachineSpec(in, out, s)
}

func Convert_v1beta1_VCDClusterStatus_To_v1alpha4_VCDClusterStatus(in *v1beta1.VCDClusterStatus, out *VCDClusterStatus, s conversion.Scope) error {
	// Todo check if _vcdcluster.status.vAppMetadata_Updated flag needs to be updated
	return autoConvert_v1beta1_VCDClusterStatus_To_v1alpha4_VCDClusterStatus(in, out, s)
}
