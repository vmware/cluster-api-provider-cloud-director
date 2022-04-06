package v1alpha4

import (
	"sigs.k8s.io/controller-runtime/pkg/conversion"
)

// ConvertTo converts this (v1alpha4)VCDCluster to the Hub version (v1alpha5).
func (src *VCDCluster) ConvertTo(dstRaw conversion.Hub) error {
	//dst := dstRaw.(*v1beta1.VCDCluster)

	//dst.Spec.ControlPlaneEnpoint := src.Spec.ControlPlaneEndpoint

	//rote conversion

	return nil
}

// ConvertFrom converts from the Hub version (v1alpha5) to this version (v1alpha4).
func (dst *VCDCluster) ConvertFrom(srcRaw conversion.Hub) error {
	//src := srcRaw.(*v1beta1.VCDCluster)
	//dst.Spec.ControlPlaneEnpoint := src.Spec.ControlPlaneEndpoint
	//rote conversion
	return nil
}
