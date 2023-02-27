package v1beta1

import (
	"github.com/vmware/cluster-api-provider-cloud-director/api/v1beta2"
	"k8s.io/apimachinery/pkg/conversion"
)

func Convert_v1beta2_VCDMachineTemplateResource_To_v1beta1_VCDMachineTemplateResource(in *v1beta2.VCDMachineTemplateResource, out *VCDMachineTemplateResource, s conversion.Scope) error {
	return autoConvert_v1beta2_VCDMachineTemplateResource_To_v1beta1_VCDMachineTemplateResource(in, out, s)
}

func Convert_v1beta2_VCDMachineSpec_To_v1beta1_VCDMachineSpec(in *v1beta2.VCDMachineSpec, out *VCDMachineSpec, s conversion.Scope) error {
	return autoConvert_v1beta2_VCDMachineSpec_To_v1beta1_VCDMachineSpec(in, out, s)
}
