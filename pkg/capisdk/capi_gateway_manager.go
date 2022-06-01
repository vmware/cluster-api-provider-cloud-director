package capisdk

import (
	"fmt"
)

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
