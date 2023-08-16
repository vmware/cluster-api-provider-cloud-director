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
