package v1beta2

import (
	"github.com/vmware/cluster-api-provider-cloud-director/api/v1beta3"
	apiconversion "k8s.io/apimachinery/pkg/conversion"
	utilconversion "sigs.k8s.io/cluster-api/util/conversion"
	"sigs.k8s.io/controller-runtime/pkg/conversion"
)

func Convert_v1beta3_VCDClusterTemplate_To_v1beta2_VCDClusterTemplate(in *v1beta3.VCDClusterTemplate, out *VCDClusterTemplate, s apiconversion.Scope) error {
	return autoConvert_v1beta3_VCDClusterTemplate_To_v1beta2_VCDClusterTemplate(in, out, s)
}

func (src *VCDClusterTemplate) ConvertTo(dstRaw conversion.Hub) error {
	dst := dstRaw.(*v1beta3.VCDClusterTemplate)
	if err := Convert_v1beta2_VCDClusterTemplate_To_v1beta3_VCDClusterTemplate(src, dst, nil); err != nil {
		return err
	}

	restored := &v1beta3.VCDClusterTemplate{}
	if ok, err := utilconversion.UnmarshalData(src, restored); err != nil || !ok {
		return err
	}
	dst.Status = restored.Status

	return nil
}

// ConvertFrom converts from the Hub version (v1beta3) to this version (v1beta2).
func (dst *VCDClusterTemplate) ConvertFrom(srcRaw conversion.Hub) error {
	src := srcRaw.(*v1beta3.VCDClusterTemplate)
	if err := Convert_v1beta3_VCDClusterTemplate_To_v1beta2_VCDClusterTemplate(src, dst, nil); err != nil {
		return err
	}

	return utilconversion.MarshalData(src, dst)
}

// ConvertTo converts this VCDMachineList to the Hub version (v1beta3).
func (src *VCDClusterTemplateList) ConvertTo(dstRaw conversion.Hub) error {
	dst := dstRaw.(*v1beta3.VCDClusterTemplateList)
	return Convert_v1beta2_VCDClusterTemplateList_To_v1beta3_VCDClusterTemplateList(src, dst, nil)
}

// ConvertFrom converts from the Hub version (v1beta3) to this version (v1beta2).
func (dst *VCDClusterTemplateList) ConvertFrom(srcRaw conversion.Hub) error {
	src := srcRaw.(*v1beta3.VCDClusterTemplateList)
	return Convert_v1beta3_VCDClusterTemplateList_To_v1beta2_VCDClusterTemplateList(src, dst, nil)
}
