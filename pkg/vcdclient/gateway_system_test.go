/*
   Copyright 2021 VMware, Inc.
   SPDX-License-Identifier: Apache-2.0
*/

/*
   Copyright 2021 VMware, Inc.
   SPDX-License-Identifier: Apache-2.0
*/

package vcdclient

import (
	"context"
	"fmt"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCacheGatewayDetails(t *testing.T) {
	vcdClient, err := getTestVCDClient(nil)
	assert.NoError(t, err, "Unable to get VCD client")
	require.NotNil(t, vcdClient, "VCD Client should not be nil")
	gateway := &GatewayManager{
		NetworkName: vcdClient.NetworkName,
		Client:      vcdClient,
	}
	ctx := context.Background()
	err = gateway.CacheGatewayDetails(ctx)

	assert.NoError(t, err, "Unable to cache gateway details")

	require.NotNil(t, vcdClient.GatewayRef, "GatewayManager reference should not be nil")
	assert.NotEmpty(t, vcdClient.GatewayRef.Name, "GatewayManager Name should not be empty")
	assert.NotEmpty(t, vcdClient.GatewayRef.Id, "GatewayManager Id should not be empty")

	// Missing network name should be reported
	vcdClient, err = getTestVCDClient(map[string]interface{}{
		"network": "",
	})
	assert.Error(t, err, "Should get error for unknown network")
	assert.Nil(t, vcdClient, "Client should be nil when erroring out")

	return
}

func TestDNATRuleCRUDE(t *testing.T) {
	vcdClient, err := getTestVCDClient(map[string]interface{}{
		"getVdcClient": true,
	})
	assert.NoError(t, err, "Unable to get VCD client")
	require.NotNil(t, vcdClient, "VCD Client should not be nil")
	gateway := &GatewayManager{
		NetworkName: vcdClient.NetworkName,
		Client:      vcdClient,
	}
	ctx := context.Background()
	err = gateway.CacheGatewayDetails(ctx)

	assert.NoError(t, err, "Unable to get VCD client")
	require.NotNil(t, vcdClient, "VCD Client should not be nil")

	dnatRuleName := fmt.Sprintf("test-dnat-rule-%s", uuid.New().String())
	err = gateway.createDNATRule(ctx, dnatRuleName, "1.2.3.4", "1.2.3.5", 80)
	assert.NoError(t, err, "Unable to create dnat rule")

	// repeated creation should not fail
	err = gateway.createDNATRule(ctx, dnatRuleName, "1.2.3.4", "1.2.3.5", 80)
	assert.NoError(t, err, "Unable to create dnat rule for the second time")

	natRuleRef, err := gateway.getNATRuleRef(ctx, dnatRuleName)
	assert.NoError(t, err, "Unable to get dnat rule")
	require.NotNil(t, natRuleRef, "Nat Rule reference should not be nil")
	assert.Equal(t, dnatRuleName, natRuleRef.Name, "Nat Rule name should match")
	assert.NotEmpty(t, natRuleRef.ID, "Nat Rule ID should not be empty")

	err = gateway.deleteDNATRule(ctx, dnatRuleName, true)
	assert.NoError(t, err, "Unable to delete dnat rule")

	err = gateway.deleteDNATRule(ctx, dnatRuleName, true)
	assert.Error(t, err, "Should fail when deleting non-existing dnat rule")

	err = gateway.deleteDNATRule(ctx, dnatRuleName, false)
	assert.NoError(t, err, "Should not fail when deleting non-existing dnat rule")

	natRuleRef, err = gateway.getNATRuleRef(ctx, dnatRuleName)
	assert.NoError(t, err, "Get should not fail when nat rule is absent")
	assert.Nil(t, natRuleRef, "Deleted nat rule reference should be nil")

	return
}

func TestLBPoolCRUDE(t *testing.T) {

	vcdClient, err := getTestVCDClient(nil)
	assert.NoError(t, err, "Unable to get VCD client")
	require.NotNil(t, vcdClient, "VCD Client should not be nil")
	gateway := &GatewayManager{
		NetworkName: vcdClient.NetworkName,
		Client:      vcdClient,
	}
	ctx := context.Background()
	err = gateway.CacheGatewayDetails(ctx)

	lbPoolName := fmt.Sprintf("test-lb-pool-%s", uuid.New().String())
	lbPoolRef, err := gateway.createLoadBalancerPool(ctx, lbPoolName, []string{"1.2.3.4", "1.2.3.5"}, 31234)
	assert.NoError(t, err, "Unable to create lb pool")
	require.NotNil(t, lbPoolRef, "LB Pool reference should not be nil")
	assert.Equal(t, lbPoolName, lbPoolRef.Name, "LB Pool name should match")

	// repeated creation should not fail
	lbPoolRef, err = gateway.createLoadBalancerPool(ctx, lbPoolName, []string{"1.2.3.4", "1.2.3.5"}, 31234)
	assert.NoError(t, err, "Unable to create lb pool for the second time")
	require.NotNil(t, lbPoolRef, "LB Pool reference should not be nil")
	assert.Equal(t, lbPoolName, lbPoolRef.Name, "LB Pool name should match")

	lbPoolRefObtained, err := gateway.getLoadBalancerPool(ctx, lbPoolName)
	assert.NoError(t, err, "Unable to get lbPool ref")
	require.NotNil(t, lbPoolRefObtained, "LB Pool reference should not be nil")
	assert.Equal(t, lbPoolRefObtained.Name, lbPoolRef.Name, "LB Pool name should match")
	assert.NotEmpty(t, lbPoolRefObtained.Id, "LB Pool ID should not be empty")

	updatedIps := []string{"5.5.5.5"}
	lbPoolRefUpdated, err := gateway.updateLoadBalancerPool(ctx, lbPoolName, updatedIps, 55555)
	assert.NoError(t, err, "No lbPool ref for updated lbPool")
	require.NotNil(t, lbPoolRefUpdated, "LB Pool reference should not be nil")
	assert.Equal(t, lbPoolRefUpdated.Name, lbPoolRef.Name, "LB Pool name should match")
	assert.NotEmpty(t, lbPoolRefUpdated.Id, "LB Pool ID should not be empty")

	lbPoolSummaryUpdated, err := gateway.getLoadBalancerPoolSummary(ctx, lbPoolName)
	assert.NoError(t, err, "No LB Pool summary for updated pool.")
	require.NotNil(t, lbPoolSummaryUpdated, "LB Pool summary reference should not be nil")
	assert.Equal(t, lbPoolSummaryUpdated.MemberCount, int32(len(updatedIps)), "LB Pool should have updated size %d", len(updatedIps))

	err = gateway.deleteLoadBalancerPool(ctx, lbPoolName, true)
	assert.NoError(t, err, "Unable to delete LB Pool")

	err = gateway.deleteLoadBalancerPool(ctx, lbPoolName, true)
	assert.Error(t, err, "Should fail when deleting non-existing lb pool")

	err = gateway.deleteLoadBalancerPool(ctx, lbPoolName, false)
	assert.NoError(t, err, "Should not fail when deleting non-existing lb pool")

	lbPoolRef, err = gateway.getLoadBalancerPool(ctx, lbPoolName)
	assert.NoError(t, err, "Get should not fail when lb pool is absent")
	assert.Nil(t, lbPoolRef, "Deleted lb pool reference should be nil")

	lbPoolRef, err = gateway.updateLoadBalancerPool(ctx, lbPoolName, updatedIps, 55555)
	assert.Error(t, err, "Updating deleted lb pool should fail")
	assert.Nil(t, lbPoolRef, "Deleted lb pool reference should be nil")

	return
}

func TestGetLoadBalancerSEG(t *testing.T) {

	vcdClient, err := getTestVCDClient(nil)
	assert.NoError(t, err, "Unable to get VCD client")
	require.NotNil(t, vcdClient, "VCD Client should not be nil")
	gateway := &GatewayManager{
		NetworkName: vcdClient.NetworkName,
		Client:      vcdClient,
	}
	ctx := context.Background()
	err = gateway.CacheGatewayDetails(ctx)

	segRef, err := gateway.getLoadBalancerSEG(ctx)
	assert.NoError(t, err, "Unable to get ServiceEngineGroup ref")
	require.NotNil(t, segRef, "ServiceEngineGroup reference should not be nil")
	assert.NotEmpty(t, segRef.Name, "ServiceEngineGroup Name should not be empty")
	assert.NotEmpty(t, segRef.Id, "ServiceEngineGroup ID should not be empty")

	return
}

func TestVirtualServiceHttpCRUDE(t *testing.T) {

	vcdClient, err := getTestVCDClient(nil)
	assert.NoError(t, err, "Unable to get VCD client")
	require.NotNil(t, vcdClient, "VCD Client should not be nil")
	gateway := &GatewayManager{
		NetworkName: vcdClient.NetworkName,
		Client:      vcdClient,
	}
	ctx := context.Background()
	err = gateway.CacheGatewayDetails(ctx)

	lbPoolName := fmt.Sprintf("test-lb-pool-%s", uuid.New().String())
	lbPoolRef, err := gateway.createLoadBalancerPool(ctx, lbPoolName, []string{"1.2.3.4", "1.2.3.5"}, 31234)
	assert.NoError(t, err, "Unable to create lb pool")

	segRef, err := gateway.getLoadBalancerSEG(ctx)
	assert.NoError(t, err, "Unable to get ServiceEngineGroup ref")

	virtualServiceName := fmt.Sprintf("test-virtual-service-%s", uuid.New().String())
	vsRef, err := gateway.createVirtualService(ctx, virtualServiceName, lbPoolRef, segRef,
		"2.3.4.5", "HTTP", 80, "")
	assert.NoError(t, err, "Unable to create virtual service")
	require.NotNil(t, vsRef, "VirtualServiceRef should not be nil")
	assert.Equal(t, virtualServiceName, vsRef.Name, "Virtual Service name should match")

	// repeated creation should not fail
	vsRef, err = gateway.createVirtualService(ctx, virtualServiceName, lbPoolRef, segRef,
		"2.3.4.5", "HTTP", 80, "")
	assert.NoError(t, err, "Unable to create virtual service for the second time")
	require.NotNil(t, vsRef, "VirtualServiceRef should not be nil")
	assert.Equal(t, virtualServiceName, vsRef.Name, "Virtual Service name should match")

	vsRefObtained, err := gateway.getVirtualService(ctx, virtualServiceName)
	assert.NoError(t, err, "Unable to get virtual service ref")
	require.NotNil(t, vsRefObtained, "Virtual service reference should not be nil")
	assert.Equal(t, vsRefObtained.Name, vsRef.Name, "Virtual service reference name should match")
	assert.NotEmpty(t, vsRefObtained.Id, "Virtual service ID should not be empty")

	err = gateway.deleteVirtualService(ctx, virtualServiceName, true)
	assert.NoError(t, err, "Unable to delete Virtual Service")

	err = gateway.deleteVirtualService(ctx, virtualServiceName, true)
	assert.Error(t, err, "Should fail when deleting non-existing Virtual Service")

	err = gateway.deleteVirtualService(ctx, virtualServiceName, false)
	assert.NoError(t, err, "Should not fail when deleting non-existing Virtual Service")

	vsRefObtained, err = gateway.getVirtualService(ctx, virtualServiceName)
	assert.NoError(t, err, "Get should not fail when Virtual Service is absent")
	assert.Nil(t, vsRefObtained, "Deleted Virtual Service reference should be nil")

	err = gateway.deleteLoadBalancerPool(ctx, lbPoolName, true)
	assert.NoError(t, err, "Should not fail when deleting lb pool")

	return
}

func TestVirtualServiceHttpsCRUDE(t *testing.T) {

	vcdClient, err := getTestVCDClient(nil)
	assert.NoError(t, err, "Unable to get VCD client")
	require.NotNil(t, vcdClient, "VCD Client should not be nil")
	gateway := &GatewayManager{
		NetworkName: vcdClient.NetworkName,
		Client:      vcdClient,
	}
	ctx := context.Background()
	err = gateway.CacheGatewayDetails(ctx)

	lbPoolName := fmt.Sprintf("test-lb-pool-%s", uuid.New().String())
	lbPoolRef, err := gateway.createLoadBalancerPool(ctx, lbPoolName, []string{"1.2.3.4", "1.2.3.5"}, 31234)
	assert.NoError(t, err, "Unable to create lb pool")

	segRef, err := gateway.getLoadBalancerSEG(ctx)
	assert.NoError(t, err, "Unable to get ServiceEngineGroup ref")

	virtualServiceName := fmt.Sprintf("test-virtual-service-https-%s", uuid.New().String())
	vsRef, err := gateway.createVirtualService(ctx, virtualServiceName, lbPoolRef, segRef,
		"2.3.4.5", "HTTPS", 443, "test")
	assert.NoError(t, err, "Unable to create virtual service")
	require.NotNil(t, vsRef, "VirtualServiceRef should not be nil")
	assert.Equal(t, virtualServiceName, vsRef.Name, "Virtual Service name should match")

	// repeated creation should not fail
	vsRef, err = gateway.createVirtualService(ctx, virtualServiceName, lbPoolRef, segRef,
		"2.3.4.5", "HTTPS", 443, "test")
	assert.NoError(t, err, "Unable to create virtual service for the second time")
	require.NotNil(t, vsRef, "VirtualServiceRef should not be nil")
	assert.Equal(t, virtualServiceName, vsRef.Name, "Virtual Service name should match")

	vsRefObtained, err := gateway.getVirtualService(ctx, virtualServiceName)
	assert.NoError(t, err, "Unable to get virtual service ref")
	require.NotNil(t, vsRefObtained, "Virtual service reference should not be nil")
	assert.Equal(t, vsRefObtained.Name, vsRef.Name, "Virtual service reference name should match")
	assert.NotEmpty(t, vsRefObtained.Id, "Virtual service ID should not be empty")

	err = gateway.deleteVirtualService(ctx, virtualServiceName, true)
	assert.NoError(t, err, "Unable to delete Virtual Service")

	err = gateway.deleteVirtualService(ctx, virtualServiceName, true)
	assert.Error(t, err, "Should fail when deleting non-existing Virtual Service")

	err = gateway.deleteVirtualService(ctx, virtualServiceName, false)
	assert.NoError(t, err, "Should not fail when deleting non-existing Virtual Service")

	vsRefObtained, err = gateway.getVirtualService(ctx, virtualServiceName)
	assert.NoError(t, err, "Get should not fail when Virtual Service is absent")
	assert.Nil(t, vsRefObtained, "Deleted Virtual Service reference should be nil")

	err = gateway.deleteLoadBalancerPool(ctx, lbPoolName, true)
	assert.NoError(t, err, "Should not fail when deleting lb pool")

	return
}

func TestLoadBalancerCRUDE(t *testing.T) {

	vcdClient, err := getTestVCDClient(nil)
	assert.NoError(t, err, "Unable to get VCD client")
	require.NotNil(t, vcdClient, "VCD Client should not be nil")
	gateway := &GatewayManager{
		NetworkName: vcdClient.NetworkName,
		Client:      vcdClient,
	}
	ctx := context.Background()
	err = gateway.CacheGatewayDetails(ctx)

	virtualServiceNamePrefix := fmt.Sprintf("test-virtual-service-https-%s", uuid.New().String())
	lbPoolNamePrefix := fmt.Sprintf("test-lb-pool-%s", uuid.New().String())
	freeIP, err := gateway.CreateLoadBalancer(ctx, virtualServiceNamePrefix,
		lbPoolNamePrefix, []string{"1.2.3.4", "1.2.3.5"}, 31234, 31235)
	assert.NoError(t, err, "Load Balancer should be created")
	assert.NotEmpty(t, freeIP, "There should be a non-empty IP returned")

	virtualServiceNameHttp := fmt.Sprintf("%s-http", virtualServiceNamePrefix)
	freeIPObtained, err := gateway.GetLoadBalancer(ctx, virtualServiceNameHttp)
	assert.NoError(t, err, "Load Balancer should be found")
	assert.Equal(t, freeIP, freeIPObtained, "The IPs should match")

	virtualServiceNameHttps := fmt.Sprintf("%s-https", virtualServiceNamePrefix)
	freeIPObtained, err = gateway.GetLoadBalancer(ctx, virtualServiceNameHttps)
	assert.NoError(t, err, "Load Balancer should be found")
	assert.Equal(t, freeIP, freeIPObtained, "The IPs should match")

	freeIP, err = gateway.CreateLoadBalancer(ctx, virtualServiceNamePrefix,
		lbPoolNamePrefix, []string{"1.2.3.4", "1.2.3.5"}, 31234, 31235)
	assert.NoError(t, err, "Load Balancer should be created even on second attempt")
	assert.NotEmpty(t, freeIP, "There should be a non-empty IP returned")

	updatedIps := []string{"5.5.5.5"}
	updatedInternalPort := int32(55555)
	err = gateway.UpdateLoadBalancer(ctx, lbPoolNamePrefix+"-http", updatedIps, updatedInternalPort)
	assert.NoError(t, err, "HTTP Load Balancer should be updated")
	err = gateway.UpdateLoadBalancer(ctx, lbPoolNamePrefix+"-https", updatedIps, updatedInternalPort)
	assert.NoError(t, err, "HTTPS Load Balancer should be updated")

	err = gateway.DeleteLoadBalancer(ctx, virtualServiceNamePrefix,
		lbPoolNamePrefix)
	assert.NoError(t, err, "Load Balancer should be deleted")

	freeIPObtained, err = gateway.GetLoadBalancer(ctx, virtualServiceNameHttp)
	assert.NoError(t, err, "Load Balancer should not be found")
	assert.Empty(t, freeIPObtained, "The VIP should not be found")

	freeIPObtained, err = gateway.GetLoadBalancer(ctx, virtualServiceNameHttps)
	assert.NoError(t, err, "Load Balancer should not be found")
	assert.Empty(t, freeIPObtained, "The VIP should not be found")

	err = gateway.DeleteLoadBalancer(ctx, virtualServiceNamePrefix,
		lbPoolNamePrefix)
	assert.NoError(t, err, "Repeated deletion of Load Balancer should not fail")

	err = gateway.UpdateLoadBalancer(ctx, lbPoolNamePrefix+"-http", updatedIps, updatedInternalPort)
	assert.Error(t, err, "updating deleted HTTP Load Balancer should be an error")
	err = gateway.UpdateLoadBalancer(ctx, lbPoolNamePrefix+"-https", updatedIps, updatedInternalPort)
	assert.Error(t, err, "updating deleted HTTPS Load Balancer should be an error")

	return
}

func TestCreateL4LoadBalancer(t *testing.T) {
	vcdClient, err := getTestVCDClient(nil)
	assert.NoError(t, err, "Unable to get VCD client")
	require.NotNil(t, vcdClient, "VCD Client should not be nil")
	gateway := &GatewayManager{
		NetworkName: vcdClient.NetworkName,
		Client:      vcdClient,
	}
	ctx := context.Background()
	err = gateway.CacheGatewayDetails(ctx)

	virtualServiceNamePrefix := fmt.Sprintf("test-virtual-service-l4-%s", uuid.New().String())
	lbPoolNamePrefix := fmt.Sprintf("test-lb-pool-%s", uuid.New().String())

	freeIP, err := gateway.CreateL4LoadBalancer(ctx, virtualServiceNamePrefix,
		lbPoolNamePrefix, []string{}, 6443)
	assert.NoError(t, err, "L4 Load Balancer should be created")
	assert.NotEmpty(t, freeIP, "There should be a non-empty IP returned")

	updatedIps := []string{"5.5.5.5"}
	updatedInternalPort := int32(55555)
	err = gateway.UpdateLoadBalancer(ctx, lbPoolNamePrefix+"-http", updatedIps, updatedInternalPort)
	assert.Error(t, err, "updating deleted HTTP Load Balancer should be an error")
}

func TestGateway_CacheGatewayDetails1(t *testing.T) {
	vcdClient, err := getTestVCDClient(nil)
	assert.NoError(t, err, "Unable to get VCD client")
	require.NotNil(t, vcdClient, "VCD Client should not be nil")
	ctx := context.Background()
	gateway := &GatewayManager{
		NetworkName: "ovdc1_nw",
		Client:      vcdClient,
	}
	gateway.CacheGatewayDetails(ctx)
	require.NotNil(t, gateway, "gateway should not be nil")

}

func TestGetLoadBalancerMemberIPs(t *testing.T) {
	vcdClient, err := getTestVCDClient(nil)
	assert.NoError(t, err, "Unable to get VCD client")
	require.NotNil(t, vcdClient, "VCD Client should not be nil")
	gateway := &GatewayManager{
		NetworkName: vcdClient.NetworkName,
		Client:      vcdClient,
	}
	ctx := context.Background()
	err = gateway.CacheGatewayDetails(ctx)

	lbPoolName := "test-lb-pool-31f7e6ec-53ae-44d4-b44a-c0e2c3d5ef7a-http"
	lbPoolRef, err := gateway.GetLoadBalancerPool(ctx, lbPoolName)
	assert.NoError(t, err, "LB pool should have been retrieved")
	memberIPs, err := gateway.GetLoadBalancerPoolMemberIPs(ctx, lbPoolRef)
	assert.NoError(t, err, "LB member IP should have been retrieved")
	updatedIps := append(memberIPs, "1.2.3.4")
	updatedInternalPort := int32(6443)
	err = gateway.UpdateLoadBalancer(ctx, lbPoolName, updatedIps, updatedInternalPort)
	assert.NoError(t, err, "HTTP Load Balancer should be updated")
}

func TestGetUnusedIPRange(t *testing.T) {
	vcdClient, err := getTestVCDClient(nil)
	assert.NoError(t, err, "Unable to get VCD client")
	require.NotNil(t, vcdClient, "VCD Client should not be nil")
	gateway := &GatewayManager{
		NetworkName: vcdClient.NetworkName,
		Client:      vcdClient,
	}
	ctx := context.Background()
	err = gateway.CacheGatewayDetails(ctx)

	freeIP, err := gateway.getUnusedExternalIPAddress(ctx, "10.150.191.253/19")
	assert.NoError(t, err, "Free IP should be retrieved.")
	assert.NotEmpty(t, freeIP, "There should be a non-empty IP returned")
}
