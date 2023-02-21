package v1alpha4

import (
	"github.com/vmware/cluster-api-provider-cloud-director/api/v1beta2"
	utilconversion "sigs.k8s.io/cluster-api/util/conversion"
	"sigs.k8s.io/controller-runtime/pkg/conversion"
)

// ConvertTo converts this (v1alpha4) VCDMachineTemplate to the Hub version (v1beta2).
func (src *VCDMachineTemplate) ConvertTo(dstRaw conversion.Hub) error {
	dst := dstRaw.(*v1beta2.VCDMachineTemplate)
	if err := Convert_v1alpha4_VCDMachineTemplate_To_v1beta2_VCDMachineTemplate(src, dst, nil); err != nil {
		return err
	}

	restored := &v1beta2.VCDMachineTemplate{}
	if ok, err := utilconversion.UnmarshalData(src, restored); err != nil || !ok {
		return err
	}

	return nil
}

// ConvertFrom converts from the Hub version (v1beta2) to this version (v1alpha4).
func (dst *VCDMachineTemplate) ConvertFrom(srcRaw conversion.Hub) error {
	src := srcRaw.(*v1beta2.VCDMachineTemplate)
	if err := Convert_v1beta2_VCDMachineTemplate_To_v1alpha4_VCDMachineTemplate(src, dst, nil); err != nil {
		return err
	}
	// add annotation "cluster.x-k8s.io/conversion-data" and return
	return utilconversion.MarshalData(src, dst)
}

// ConvertTo converts this VCDMachineTemplateList to the Hub version (v1beta2).
func (src *VCDMachineTemplateList) ConvertTo(dstRaw conversion.Hub) error {
	dst := dstRaw.(*v1beta2.VCDMachineTemplateList)
	return Convert_v1alpha4_VCDMachineTemplateList_To_v1beta2_VCDMachineTemplateList(src, dst, nil)
}

// ConvertFrom converts from the Hub version (v1beta2) to this version (v1alpha4).
func (dst *VCDMachineTemplateList) ConvertFrom(srcRaw conversion.Hub) error {
	src := srcRaw.(*v1beta2.VCDMachineTemplateList)
	return Convert_v1beta2_VCDMachineTemplateList_To_v1alpha4_VCDMachineTemplateList(src, dst, nil)
}
