package v1alpha4

import (
	//"github.com/vmware/cluster-api-provider-cloud-director/api/v1beta1"
	"github.com/vmware/cluster-api-provider-cloud-director/api/v1beta1"
	utilconversion "sigs.k8s.io/cluster-api/util/conversion"
	"sigs.k8s.io/controller-runtime/pkg/conversion"
)

// ConvertTo converts this (v1alpha4)VCDCluster to the Hub version (v1beta1).
func (src *VCDMachineTemplate) ConvertTo(dstRaw conversion.Hub) error {
	dst := dstRaw.(*v1beta1.VCDMachineTemplate)

	if err := Convert_v1alpha4_VCDMachineTemplate_To_v1beta1_VCDMachineTemplate(src, dst, nil); err != nil {
		return err
	}
	return nil
}

// ConvertFrom converts from the Hub version (v1beta1) to this version (v1alpha4).
func (dst *VCDMachineTemplate) ConvertFrom(srcRaw conversion.Hub) error {
	src := srcRaw.(*v1beta1.VCDMachineTemplate)
	if err := Convert_v1beta1_VCDMachineTemplate_To_v1alpha4_VCDMachineTemplate(src, dst, nil); err != nil {
		return err
	}
	// add annotation "cluster.x-k8s.io/conversion-data" and return
	return utilconversion.MarshalData(src, dst)
}

// ConvertTo converts this VCDClusterList to the Hub version (v1beta1).
func (src *VCDMachineTemplateList) ConvertTo(dstRaw conversion.Hub) error {
	dst := dstRaw.(*v1beta1.VCDMachineTemplateList)
	return Convert_v1alpha4_VCDMachineTemplateList_To_v1beta1_VCDMachineTemplateList(src, dst, nil)
}

// ConvertFrom converts from the Hub version (v1beta1) to this version (v1alpha4).
func (dst *VCDMachineTemplateList) ConvertFrom(srcRaw conversion.Hub) error {
	src := srcRaw.(*v1beta1.VCDMachineTemplateList)
	return Convert_v1beta1_VCDMachineTemplateList_To_v1alpha4_VCDMachineTemplateList(src, dst, nil)
}
