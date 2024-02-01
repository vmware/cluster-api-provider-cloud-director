package v1alpha4

import (
	"github.com/vmware/cluster-api-provider-cloud-director/api/v1beta3"
	utilconversion "sigs.k8s.io/cluster-api/util/conversion"
	"sigs.k8s.io/controller-runtime/pkg/conversion"
)

// ConvertTo converts this (v1alpha4) VCDMachine to the Hub version (v1beta3).
func (src *VCDMachine) ConvertTo(dstRaw conversion.Hub) error {
	dst := dstRaw.(*v1beta3.VCDMachine)
	if err := Convert_v1alpha4_VCDMachine_To_v1beta3_VCDMachine(src, dst, nil); err != nil {
		return err
	}

	// there is a possibility that the older version (v1alpha4) won't have the "cluster.x-k8s.io/conversion-data" annotation.
	// so use the src object to recover fields which are necessary in the new version
	dst.Spec.SizingPolicy = src.Spec.ComputePolicy
	dst.Status.ProviderID = src.Spec.ProviderID

	// manually restore data
	restored := &v1beta3.VCDMachine{}
	if ok, err := utilconversion.UnmarshalData(src, restored); err != nil || !ok {
		// in the case of missing v1beta3 annotation, the return value of UnmarshalData() would be (false, nil)
		// so the return value would be nil NOT err
		return err
	}

	// restore all the fields, even those which were derived using the src object, using the "cluster.x-k8s.io/conversion-data" annotation
	// Also note that dst and restored are of the same type
	dst.Spec.SizingPolicy = restored.Spec.SizingPolicy
	dst.Spec.PlacementPolicy = restored.Spec.PlacementPolicy
	dst.Spec.StorageProfile = restored.Spec.StorageProfile
	dst.Spec.DiskSize = restored.Spec.DiskSize
	dst.Spec.EnableNvidiaGPU = restored.Spec.EnableNvidiaGPU
	dst.Spec.ExtraOvdcNetworks = restored.Spec.ExtraOvdcNetworks
	dst.Spec.VmNamingTemplate = restored.Spec.VmNamingTemplate
	dst.Spec.FailureDomain = restored.Spec.FailureDomain

	dst.Status.Template = restored.Status.Template
	dst.Status.ProviderID = restored.Status.ProviderID
	dst.Status.SizingPolicy = restored.Status.SizingPolicy
	dst.Status.PlacementPolicy = restored.Status.PlacementPolicy
	dst.Status.NvidiaGPUEnabled = restored.Status.NvidiaGPUEnabled
	dst.Status.DiskSize = restored.Status.DiskSize
	return nil
}

// ConvertFrom converts from the Hub version (v1beta3) to this version (v1alpha4).
func (dst *VCDMachine) ConvertFrom(srcRaw conversion.Hub) error {
	src := srcRaw.(*v1beta3.VCDMachine)
	if err := Convert_v1beta3_VCDMachine_To_v1alpha4_VCDMachine(src, dst, nil); err != nil {
		return err
	}

	dst.Spec.ComputePolicy = src.Spec.SizingPolicy
	// add annotation "cluster.x-k8s.io/conversion-data" and return
	return utilconversion.MarshalData(src, dst)
}

// ConvertTo converts this VCDMachineList to the Hub version (v1beta3).
func (src *VCDMachineList) ConvertTo(dstRaw conversion.Hub) error {
	dst := dstRaw.(*v1beta3.VCDMachineList)
	return Convert_v1alpha4_VCDMachineList_To_v1beta3_VCDMachineList(src, dst, nil)
}

// ConvertFrom converts from the Hub version (v1beta3) to this version (v1alpha4).
func (dst *VCDMachineList) ConvertFrom(srcRaw conversion.Hub) error {
	src := srcRaw.(*v1beta3.VCDMachineList)
	return Convert_v1beta3_VCDMachineList_To_v1alpha4_VCDMachineList(src, dst, nil)
}
