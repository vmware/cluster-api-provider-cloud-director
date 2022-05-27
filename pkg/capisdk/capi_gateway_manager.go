package capisdk

import (
	"fmt"
	"github.com/vmware/cloud-provider-for-cloud-director/pkg/vcdsdk"
)

type CapvcdGatewayManager struct {
	gatewayManager *vcdsdk.GatewayManager
}

func GetVirtualServiceNamePrefix(clusterName string, clusterID string) string {
	return clusterName + "-" + clusterID
}

func GetLoadBalancerPoolNamePrefix(clusterName string, clusterID string) string {
	return clusterName + "-" + clusterID
}

func GetVirtualServiceNameUsingPrefix(virtualServiceNamePrefix string, portSuffix string) string {
	return fmt.Sprintf("%s-%s", virtualServiceNamePrefix, portSuffix)
}

func GetLoadBalancerPoolNameUsingPrefix(lbPoolNamePrefix string, portSuffix string) string {
	return fmt.Sprintf("%s-%s", lbPoolNamePrefix, portSuffix)
}
