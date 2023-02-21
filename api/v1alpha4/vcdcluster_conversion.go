package v1alpha4

import (
	"github.com/vmware/cloud-provider-for-cloud-director/pkg/vcdsdk"
	"github.com/vmware/cluster-api-provider-cloud-director/api/v1beta2"
	utilconversion "sigs.k8s.io/cluster-api/util/conversion"
	"sigs.k8s.io/controller-runtime/pkg/conversion"
	"strings"
)

// ConvertTo converts this (v1alpha4) VCDCluster to the Hub version (v1beta2).
func (src *VCDCluster) ConvertTo(dstRaw conversion.Hub) error {
	dst := dstRaw.(*v1beta2.VCDCluster)
	if err := Convert_v1alpha4_VCDCluster_To_v1beta2_VCDCluster(src, dst, nil); err != nil {
		return err
	}
	// there is a possibility that the older version (v1alpha4) won't have the "cluster.x-k8s.io/conversion-data" annotation.
	// so use the src object to recover fields which are necessary in the new version
	dst.Spec.RDEId = src.Status.InfraId
	if strings.HasPrefix(src.Status.InfraId, vcdsdk.NoRdePrefix) {
		dst.Status.RdeVersionInUse = vcdsdk.NoRdePrefix
	} else {
		dst.Status.RdeVersionInUse = "1.0.0" // value will be checked by vcdcluster controller if RDE upgrade is necessary
	}
	// In v1alpha4 DNAT rules (and one-arm) are used by default. Therefore, use that in v1beta2
	dst.Spec.LoadBalancerConfigSpec.UseOneArm = true

	// manually restore data
	restored := &v1beta2.VCDCluster{}
	if ok, err := utilconversion.UnmarshalData(src, restored); err != nil || !ok {
		// in the case of missing v1beta2 annotation, the return value of UnmarshalData() would be (false, nil)
		// so the return value would be nil NOT err
		return err
	}

	// restore all the fields, even those which were derived using the src object, using the "cluster.x-k8s.io/conversion-data" annotation
	// Also note that dst and restored are of the same type
	dst.Spec.RDEId = restored.Spec.RDEId
	dst.Status.RdeVersionInUse = restored.Status.RdeVersionInUse
	dst.Spec.ProxyConfigSpec = restored.Spec.ProxyConfigSpec
	dst.Spec.ParentUID = restored.Spec.ParentUID
	dst.Spec.UseAsManagementCluster = restored.Spec.UseAsManagementCluster // defaults to false
	dst.Status.RdeVersionInUse = restored.Status.RdeVersionInUse
	dst.Spec.LoadBalancerConfigSpec.UseOneArm = restored.Spec.LoadBalancerConfigSpec.UseOneArm
	dst.Spec.LoadBalancerConfigSpec.VipSubnet = restored.Spec.LoadBalancerConfigSpec.VipSubnet
	return nil
}

// ConvertFrom converts from the Hub version (v1beta2) to this version (v1alpha4).
func (dst *VCDCluster) ConvertFrom(srcRaw conversion.Hub) error {
	src := srcRaw.(*v1beta2.VCDCluster)
	if err := Convert_v1beta2_VCDCluster_To_v1alpha4_VCDCluster(src, dst, nil); err != nil {
		return err
	}
	// add annotation "cluster.x-k8s.io/conversion-data" and return
	return utilconversion.MarshalData(src, dst)
}

// ConvertTo converts this VCDClusterList to the Hub version (v1beta2).
func (src *VCDClusterList) ConvertTo(dstRaw conversion.Hub) error {
	dst := dstRaw.(*v1beta2.VCDClusterList)
	return Convert_v1alpha4_VCDClusterList_To_v1beta2_VCDClusterList(src, dst, nil)
}

// ConvertFrom converts from the Hub version (v1beta2) to this version (v1alpha4).
func (dst *VCDClusterList) ConvertFrom(srcRaw conversion.Hub) error {
	src := srcRaw.(*v1beta2.VCDClusterList)
	return Convert_v1beta2_VCDClusterList_To_v1alpha4_VCDClusterList(src, dst, nil)
}
