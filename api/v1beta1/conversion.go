package v1beta1

import (
	"github.com/vmware/cluster-api-provider-cloud-director/api/v1beta3"
	"k8s.io/apimachinery/pkg/conversion"
)

func Convert_v1beta3_VCDMachineTemplateResource_To_v1beta1_VCDMachineTemplateResource(in *v1beta3.VCDMachineTemplateResource, out *VCDMachineTemplateResource, s conversion.Scope) error {
	return autoConvert_v1beta3_VCDMachineTemplateResource_To_v1beta1_VCDMachineTemplateResource(in, out, s)
}

func Convert_v1beta3_VCDMachineSpec_To_v1beta1_VCDMachineSpec(in *v1beta3.VCDMachineSpec, out *VCDMachineSpec, s conversion.Scope) error {
	return autoConvert_v1beta3_VCDMachineSpec_To_v1beta1_VCDMachineSpec(in, out, s)
}

func Convert_v1beta3_VCDClusterSpec_To_v1beta1_VCDClusterSpec(in *v1beta3.VCDClusterSpec, out *VCDClusterSpec, s conversion.Scope) error {
	return autoConvert_v1beta3_VCDClusterSpec_To_v1beta1_VCDClusterSpec(in, out, s)
}

func Convert_v1beta1_VCDClusterSpec_To_v1beta3_VCDClusterSpec(in *VCDClusterSpec, out *v1beta3.VCDClusterSpec, s conversion.Scope) error {
	return autoConvert_v1beta1_VCDClusterSpec_To_v1beta3_VCDClusterSpec(in, out, s)
}

func Convert_v1beta3_VCDClusterStatus_To_v1beta1_VCDClusterStatus(in *v1beta3.VCDClusterStatus, out *VCDClusterStatus, s conversion.Scope) error {
	return autoConvert_v1beta3_VCDClusterStatus_To_v1beta1_VCDClusterStatus(in, out, s)
}

func Convert_v1beta3_VCDMachineStatus_To_v1beta1_VCDMachineStatus(in *v1beta3.VCDMachineStatus, out *VCDMachineStatus, s conversion.Scope) error {
	return autoConvert_v1beta3_VCDMachineStatus_To_v1beta1_VCDMachineStatus(in, out, s)
}
