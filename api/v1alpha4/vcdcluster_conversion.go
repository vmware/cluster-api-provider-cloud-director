package v1alpha4

import (
	"github.com/vmware/cluster-api-provider-cloud-director/api/v1beta1"
	"sigs.k8s.io/controller-runtime/pkg/conversion"
)

// ConvertTo converts this (v1alpha4)VCDCluster to the Hub version (v1beta1).
func (src *VCDCluster) ConvertTo(dstRaw conversion.Hub) error {
	dst := dstRaw.(*v1beta1.VCDCluster)
	if err := Convert_v1alpha4_VCDCluster_To_v1beta1_VCDCluster(src, dst, nil); err != nil {
		return err
	}
	dst.Spec.DefaultStorageClassOptions = &v1beta1.DefaultStorageClassOptions{}
	dst.Spec.RDEId = ""
	dst.Spec.ParentUID = ""
	dst.Spec.UseAsManagementCluster = false // defaults to false
	return nil
}

// ConvertFrom converts from the Hub version (v1beta1) to this version (v1alpha4).
func (dst *VCDCluster) ConvertFrom(srcRaw conversion.Hub) error {
	src := srcRaw.(*v1beta1.VCDCluster)
	if err := Convert_v1beta1_VCDCluster_To_v1alpha4_VCDCluster(src, dst, nil); err != nil {
		return err
	}
	return nil
}

// ConvertTo converts this VCDClusterList to the Hub version (v1beta1).
func (src *VCDClusterList) ConvertTo(dstRaw conversion.Hub) error {
	dst := dstRaw.(*v1beta1.VCDClusterList)
	return Convert_v1alpha4_VCDClusterList_To_v1beta1_VCDClusterList(src, dst, nil)
}

// ConvertFrom converts from the Hub version (v1beta1) to this version (v1alpha4).
func (dst *VCDClusterList) ConvertFrom(srcRaw conversion.Hub) error {
	src := srcRaw.(*v1beta1.VCDClusterList)
	return Convert_v1beta1_VCDClusterList_To_v1alpha4_VCDClusterList(src, dst, nil)
}
