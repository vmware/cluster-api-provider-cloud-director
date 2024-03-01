package v1beta2

import (
	"github.com/vmware/cluster-api-provider-cloud-director/api/v1beta3"
	utilconversion "sigs.k8s.io/cluster-api/util/conversion"
	"sigs.k8s.io/controller-runtime/pkg/conversion"
)

// ConvertTo converts VCDClusterTemplate to the Hub version (v1beta3).
func (src *VCDClusterTemplate) ConvertTo(dstRaw conversion.Hub) error {
	dst := dstRaw.(*v1beta3.VCDClusterTemplate)
	if err := Convert_v1beta2_VCDClusterTemplate_To_v1beta3_VCDClusterTemplate(src, dst, nil); err != nil {
		return err
	}

	restored := &v1beta3.VCDClusterTemplate{}
	if ok, err := utilconversion.UnmarshalData(src, restored); err != nil || !ok {
		return err
	}

	return nil
}

// ConvertFrom converts VCDClusterTemplate from the Hub version (v1beta3) to this version (v1beta2).
func (dst *VCDClusterTemplate) ConvertFrom(srcRaw conversion.Hub) error {
	src := srcRaw.(*v1beta3.VCDClusterTemplate)
	if err := Convert_v1beta3_VCDClusterTemplate_To_v1beta2_VCDClusterTemplate(src, dst, nil); err != nil {
		return err
	}

	return utilconversion.MarshalData(src, dst)
}

// ConvertTo converts VCDClusterTemplateList to the Hub version (v1beta3).
func (src *VCDClusterTemplateList) ConvertTo(dstRaw conversion.Hub) error {
	dst := dstRaw.(*v1beta3.VCDClusterTemplateList)
	return Convert_v1beta2_VCDClusterTemplateList_To_v1beta3_VCDClusterTemplateList(src, dst, nil)
}

// ConvertFrom converts VCDClusterTemplateList from the Hub version (v1beta3) to this version (v1beta2).
func (dst *VCDClusterTemplateList) ConvertFrom(srcRaw conversion.Hub) error {
	src := srcRaw.(*v1beta3.VCDClusterTemplateList)
	return Convert_v1beta3_VCDClusterTemplateList_To_v1beta2_VCDClusterTemplateList(src, dst, nil)
}
