package v1alpha4

import (
	"github.com/vmware/cluster-api-provider-cloud-director/api/v1beta1"
	utilconversion "sigs.k8s.io/cluster-api/util/conversion"
	"sigs.k8s.io/controller-runtime/pkg/conversion"
)

// ConvertTo converts this (v1alpha4)VCDCluster to the Hub version (v1beta1).
func (src *VCDMachine) ConvertTo(dstRaw conversion.Hub) error {
	dst := dstRaw.(*v1beta1.VCDMachine)
	if err := Convert_v1alpha4_VCDMachine_To_v1beta1_VCDMachine(src, dst, nil); err != nil {
		return err
	}

	// there is a possibility that the older version (v1alpha4) won't have the "cluster.x-k8s.io/conversion-data" annotation.
	// so use the src object to recover fields which are necessary in the new version
	dst.Spec.SizingPolicy = src.Spec.ComputePolicy
	dst.Status.ProviderID = src.Spec.ProviderID

	// manually restore data
	restored := v1beta1.VCDMachine{}
	if ok, err := utilconversion.UnmarshalData(src, restored); err != nil || !ok {
		return err
	}

	// restore all the fields, even those which were derived using the src object, using the "cluster.x-k8s.io/conversion-data" annotation
	dst.Spec.SizingPolicy = restored.Spec.SizingPolicy
	dst.Spec.StorageProfile = restored.Spec.StorageProfile
	dst.Status.Template = restored.Status.Template
	dst.Status.ProviderID = restored.Spec.ProviderID
	return nil
}

// ConvertFrom converts from the Hub version (v1beta1) to this version (v1alpha4).
func (dst *VCDMachine) ConvertFrom(srcRaw conversion.Hub) error {
	src := srcRaw.(*v1beta1.VCDMachine)
	if err := Convert_v1beta1_VCDMachine_To_v1alpha4_VCDMachine(src, dst, nil); err != nil {
		return err
	}

	dst.Spec.ComputePolicy = src.Spec.SizingPolicy
	// add annotation "cluster.x-k8s.io/conversion-data" and return
	return utilconversion.MarshalData(src, dst)
}

// ConvertTo converts this VCDMachineList to the Hub version (v1beta1).
func (src *VCDMachineList) ConvertTo(dstRaw conversion.Hub) error {
	dst := dstRaw.(*v1beta1.VCDMachineList)
	return Convert_v1alpha4_VCDMachineList_To_v1beta1_VCDMachineList(src, dst, nil)
}

// ConvertFrom converts from the Hub version (v1beta1) to this version (v1alpha4).
func (dst *VCDMachineList) ConvertFrom(srcRaw conversion.Hub) error {
	src := srcRaw.(*v1beta1.VCDMachineList)
	return Convert_v1beta1_VCDMachineList_To_v1alpha4_VCDMachineList(src, dst, nil)
}
