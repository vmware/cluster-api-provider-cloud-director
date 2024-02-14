package v1beta2

import (
	"github.com/vmware/cluster-api-provider-cloud-director/api/v1beta3"
	utilconversion "sigs.k8s.io/cluster-api/util/conversion"
	"sigs.k8s.io/controller-runtime/pkg/conversion"
)

// ConvertTo converts this VCDMachineTemplate to the Hub version (v1beta3).
func (src *VCDMachineTemplate) ConvertTo(dstRaw conversion.Hub) error {
	dst := dstRaw.(*v1beta3.VCDMachineTemplate)
	if err := Convert_v1beta2_VCDMachineTemplate_To_v1beta3_VCDMachineTemplate(src, dst, nil); err != nil {
		return err
	}

	restored := &v1beta3.VCDMachineTemplate{}
	if ok, err := utilconversion.UnmarshalData(src, restored); err != nil || !ok {
		return err
	}
	dst.Status = restored.Status
	return nil
}

// ConvertFrom converts from the Hub version (v1beta3) to this version (v1beta2).
func (dst *VCDMachineTemplate) ConvertFrom(srcRaw conversion.Hub) error {
	src := srcRaw.(*v1beta3.VCDMachineTemplate)
	if err := Convert_v1beta3_VCDMachineTemplate_To_v1beta2_VCDMachineTemplate(src, dst, nil); err != nil {
		return err
	}
	return utilconversion.MarshalData(src, dst)
}

// ConvertTo converts this VCDMachineTemplateList to the Hub version (v1beta3).
func (src *VCDMachineTemplateList) ConvertTo(dstRaw conversion.Hub) error {
	dst := dstRaw.(*v1beta3.VCDMachineTemplateList)
	return Convert_v1beta2_VCDMachineTemplateList_To_v1beta3_VCDMachineTemplateList(src, dst, nil)
}

// ConvertFrom converts from the Hub version (v1beta3) to this version (v1beta2).
func (dst *VCDMachineTemplateList) ConvertFrom(srcRaw conversion.Hub) error {
	src := srcRaw.(*v1beta3.VCDMachineTemplateList)
	return Convert_v1beta3_VCDMachineTemplateList_To_v1beta2_VCDMachineTemplateList(src, dst, nil)
}
