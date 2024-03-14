package v1beta1

import (
	"github.com/vmware/cluster-api-provider-cloud-director/api/v1beta3"
	utilconversion "sigs.k8s.io/cluster-api/util/conversion"
	"sigs.k8s.io/controller-runtime/pkg/conversion"
)

// ConvertTo converts this VCDCluster to the Hub version (v1beta3).
func (src *VCDCluster) ConvertTo(dstRaw conversion.Hub) error {
	dst := dstRaw.(*v1beta3.VCDCluster)
	if err := Convert_v1beta1_VCDCluster_To_v1beta3_VCDCluster(src, dst, nil); err != nil {
		return err
	}

	restored := &v1beta3.VCDCluster{}
	if ok, err := utilconversion.UnmarshalData(src, restored); err != nil || !ok {
		return err
	}
	dst.Spec.MultiZoneSpec.ZoneTopology = restored.Spec.MultiZoneSpec.ZoneTopology
	dst.Spec.MultiZoneSpec.DCGroupConfig = restored.Spec.MultiZoneSpec.DCGroupConfig
	dst.Spec.MultiZoneSpec.Zones = restored.Spec.MultiZoneSpec.Zones

	dst.Status.VcdResourceMap = restored.Status.VcdResourceMap
	dst.Status.MultiZoneStatus.ZoneTopology = restored.Status.MultiZoneStatus.ZoneTopology
	dst.Status.MultiZoneStatus.DCGroupConfig = restored.Status.MultiZoneStatus.DCGroupConfig
	dst.Status.MultiZoneStatus.Zones = restored.Status.MultiZoneStatus.Zones
	return nil
}

// ConvertFrom converts from the Hub version (v1beta3) to this version (v1beta1).
func (dst *VCDCluster) ConvertFrom(srcRaw conversion.Hub) error {
	src := srcRaw.(*v1beta3.VCDCluster)
	if err := Convert_v1beta3_VCDCluster_To_v1beta1_VCDCluster(src, dst, nil); err != nil {
		return err
	}
	return utilconversion.MarshalData(src, dst)
}

// ConvertTo converts this VCDClusterList to the Hub version (v1beta3).
func (src *VCDClusterList) ConvertTo(dstRaw conversion.Hub) error {
	dst := dstRaw.(*v1beta3.VCDClusterList)
	return Convert_v1beta1_VCDClusterList_To_v1beta3_VCDClusterList(src, dst, nil)
}

// ConvertFrom converts from the Hub version (v1beta3) to this version (v1beta1).
func (dst *VCDClusterList) ConvertFrom(srcRaw conversion.Hub) error {
	src := srcRaw.(*v1beta3.VCDClusterList)
	return Convert_v1beta3_VCDClusterList_To_v1beta1_VCDClusterList(src, dst, nil)
}
