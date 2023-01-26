package v1beta1

import (
	"github.com/vmware/cluster-api-provider-cloud-director/api/v1beta2"
	"sigs.k8s.io/controller-runtime/pkg/conversion"
)

// ConvertTo converts this VCDMachine to the Hub version (v1beta2).
func (src *VCDMachine) ConvertTo(dstRaw conversion.Hub) error {
	dst := dstRaw.(*v1beta2.VCDMachine)
	return Convert_v1beta1_VCDMachine_To_v1beta2_VCDMachine(src, dst, nil)
}

// ConvertFrom converts from the Hub version (v1beta2) to this version (v1beta1).
func (dst *VCDMachine) ConvertFrom(srcRaw conversion.Hub) error {
	src := srcRaw.(*v1beta2.VCDMachine)
	return Convert_v1beta2_VCDMachine_To_v1beta1_VCDMachine(src, dst, nil)
}

// ConvertTo converts this VCDMachineList to the Hub version (v1beta2).
func (src *VCDMachineList) ConvertTo(dstRaw conversion.Hub) error {
	dst := dstRaw.(*v1beta2.VCDMachineList)
	return Convert_v1beta1_VCDMachineList_To_v1beta2_VCDMachineList(src, dst, nil)
}

// ConvertFrom converts from the Hub version (v1beta2) to this version (v1beta1).
func (dst *VCDMachineList) ConvertFrom(srcRaw conversion.Hub) error {
	src := srcRaw.(*v1beta2.VCDMachineList)
	return Convert_v1beta2_VCDMachineList_To_v1beta1_VCDMachineList(src, dst, nil)
}
