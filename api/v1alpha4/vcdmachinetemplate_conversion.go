package v1alpha4

import (
	"github.com/vmware/cluster-api-provider-cloud-director/api/v1beta1"
	infrav1beta1 "sigs.k8s.io/cluster-api-pro"
	"sigs.k8s.io/controller-runtime/pkg/conversion"
)

// ConvertTo converts this (v1alpha4)VCDCluster to the Hub version (v1alpha5).
func (src *VCDMachineTemplate) ConvertTo(dstRaw conversion.Hub) error {
	dst := dstRaw.(*v1beta1.VCDMachineTemplate)

	restored := &v1beta1.VCDMachineTemplate{}

	//dst.Spec.ControlPlaneEnpoint := src.Spec.ControlPlaneEndpoint

	//rote conversion
	return nil
}

// ConvertFrom converts from the Hub version (v1alpha5) to this version (v1alpha4).
func (dst *VCDMachineTemplate) ConvertFrom(srcRaw conversion.Hub) error {
	//src := srcRaw.(*v1beta1.VCDCluster)
	//dst.Spec.ControlPlaneEnpoint := src.Spec.ControlPlaneEndpoint
	//rote conversion
	return nil
}
