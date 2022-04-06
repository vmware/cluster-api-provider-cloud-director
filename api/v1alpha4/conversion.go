package v1alpha4

import (
	"github.com/vmware/cluster-api-provider-cloud-director/api/v1beta1"
	"k8s.io/apimachinery/pkg/conversion"
)

func Convert_v1beta1_VCDClusterSpec_To_v1alpha4_VCDClusterSpec(in *v1beta1.VCDClusterSpec, out *VCDClusterSpec, s conversion.Scope) error {
	return autoConvert_v1beta1_VCDClusterSpec_To_v1alpha4_VCDClusterSpec(in, out, s)
}
