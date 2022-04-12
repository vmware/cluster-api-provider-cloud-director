package v1alpha4

import (
	"github.com/vmware/cluster-api-provider-cloud-director/api/v1beta1"
	"sigs.k8s.io/controller-runtime/pkg/conversion"
)

// ConvertTo converts this (v1alpha4)VCDCluster to the Hub version (v1beta1).
func (src *VCDMachine) ConvertTo(dstRaw conversion.Hub) error {
	dst := dstRaw.(*v1beta1.VCDMachine)

	if err := Convert_v1alpha4_VCDMachine_To_v1beta1_VCDMachine(src, dst, nil); err != nil {
		return err
	}

	return nil
}

// ConvertFrom converts from the Hub version (v1beta1) to this version (v1alpha4).
func (dst *VCDMachine) ConvertFrom(srcRaw conversion.Hub) error {
	src := srcRaw.(*v1beta1.VCDMachine)
	if err := Convert_v1beta1_VCDMachine_To_v1alpha4_VCDMachine(src, dst, nil); err != nil {
		return err
	}
	return nil
}

// ConvertTo converts this VCDMachineList to the Hub version (v1beta1).
func (src *VCDMachineList) ConvertTo(dstRaw conversion.Hub) error {
	dst := dstRaw.(*v1beta1.VCDMachineList)
	return Convert_v1alpha4_VCDMachineList_To_v1beta1_VCDMachineList(src, dst, nil)
}

// ConvertFrom converts from the Hub version (v1beta1) to this version (v1alpha4).
func (dst *VCDMachineList) ConvertFrom(srcRaw conversion.Hub) error {
	src := srcRaw.(*v1beta1.VCDMachineList)
	return Convert_v1beta1_VCDMachineList_To_v1alpha4_VCDMachineList(src, dst, nil)
}
