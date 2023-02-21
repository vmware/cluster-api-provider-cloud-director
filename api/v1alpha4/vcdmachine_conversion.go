package v1alpha4

import (
	"github.com/vmware/cluster-api-provider-cloud-director/api/v1beta2"
	utilconversion "sigs.k8s.io/cluster-api/util/conversion"
	"sigs.k8s.io/controller-runtime/pkg/conversion"
)

// ConvertTo converts this (v1alpha4) VCDMachine to the Hub version (v1beta2).
func (src *VCDMachine) ConvertTo(dstRaw conversion.Hub) error {
	dst := dstRaw.(*v1beta2.VCDMachine)
	if err := Convert_v1alpha4_VCDMachine_To_v1beta2_VCDMachine(src, dst, nil); err != nil {
		return err
	}

	// there is a possibility that the older version (v1alpha4) won't have the "cluster.x-k8s.io/conversion-data" annotation.
	// so use the src object to recover fields which are necessary in the new version
	dst.Spec.SizingPolicy = src.Spec.ComputePolicy
	dst.Status.ProviderID = src.Spec.ProviderID

	// manually restore data
	restored := &v1beta2.VCDMachine{}
	if ok, err := utilconversion.UnmarshalData(src, restored); err != nil || !ok {
		// in the case of missing v1beta2 annotation, the return value of UnmarshalData() would be (false, nil)
		// so the return value would be nil NOT err
		return err
	}

	// restore all the fields, even those which were derived using the src object, using the "cluster.x-k8s.io/conversion-data" annotation
	// Also note that dst and restored are of the same type
	dst.Spec.SizingPolicy = restored.Spec.SizingPolicy
	dst.Spec.StorageProfile = restored.Spec.StorageProfile
	dst.Status.Template = restored.Status.Template
	dst.Status.ProviderID = restored.Spec.ProviderID
	return nil
}

// ConvertFrom converts from the Hub version (v1beta2) to this version (v1alpha4).
func (dst *VCDMachine) ConvertFrom(srcRaw conversion.Hub) error {
	src := srcRaw.(*v1beta2.VCDMachine)
	if err := Convert_v1beta2_VCDMachine_To_v1alpha4_VCDMachine(src, dst, nil); err != nil {
		return err
	}

	dst.Spec.ComputePolicy = src.Spec.SizingPolicy
	// add annotation "cluster.x-k8s.io/conversion-data" and return
	return utilconversion.MarshalData(src, dst)
}

// ConvertTo converts this VCDMachineList to the Hub version (v1beta2).
func (src *VCDMachineList) ConvertTo(dstRaw conversion.Hub) error {
	dst := dstRaw.(*v1beta2.VCDMachineList)
	return Convert_v1alpha4_VCDMachineList_To_v1beta2_VCDMachineList(src, dst, nil)
}

// ConvertFrom converts from the Hub version (v1beta2) to this version (v1alpha4).
func (dst *VCDMachineList) ConvertFrom(srcRaw conversion.Hub) error {
	src := srcRaw.(*v1beta2.VCDMachineList)
	return Convert_v1beta2_VCDMachineList_To_v1alpha4_VCDMachineList(src, dst, nil)
}
