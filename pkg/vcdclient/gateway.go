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
	"net"
	"net/http"
	"net/url"
	"strconv"

	"github.com/apparentlymart/go-cidr/cidr"
	"github.com/peterhellberg/link"

	"github.com/antihax/optional"
	swaggerClient "github.com/vmware/cluster-api-provider-cloud-director/pkg/vcdswaggerclient"
	"github.com/vmware/go-vcloud-director/v2/govcd"
	"k8s.io/klog"
)

type GatewayManager struct {
	NetworkName        string
	GatewayRef         *swaggerClient.EntityReference
	NetworkBackingType swaggerClient.BackingNetworkType
	Client             *Client
}

// CacheGatewayDetails : get gateway reference and cache some details in client object
func (gateway *GatewayManager) CacheGatewayDetails(ctx context.Context) error {

	if gateway.NetworkName == "" {
		return fmt.Errorf("network name should not be empty")
	}

	ovdcNetworksAPI := gateway.Client.ApiClient.OrgVdcNetworksApi
	pageNum := int32(1)
	ovdcNetworkID := ""
	for {
		ovdcNetworks, resp, err := ovdcNetworksAPI.GetAllVdcNetworks(ctx, pageNum, 32, nil)
		if err != nil {
			// TODO: log resp in debug mode only
			return fmt.Errorf("unable to get all ovdc networks: [%+v]: [%v]", resp, err)
		}

		if len(ovdcNetworks.Values) == 0 {
			break
		}

		for _, ovdcNetwork := range ovdcNetworks.Values {
			if ovdcNetwork.Name == gateway.NetworkName {
				ovdcNetworkID = ovdcNetwork.Id
				break
			}
		}

		if ovdcNetworkID != "" {
			break
		}
		pageNum++
	}
	if ovdcNetworkID == "" {
		return fmt.Errorf("unable to obtain ID for ovdc network name [%s]",
			gateway.NetworkName)
	}

	ovdcNetworkAPI := gateway.Client.ApiClient.OrgVdcNetworkApi
	ovdcNetwork, resp, err := ovdcNetworkAPI.GetOrgVdcNetwork(ctx, ovdcNetworkID)
	if err != nil {
		return fmt.Errorf("unable to get network for id [%s]: [%+v]: [%v]", ovdcNetworkID, resp, err)
	}

	// Cache gateway reference
	gateway.GatewayRef = &swaggerClient.EntityReference{
		Name: ovdcNetwork.Connection.RouterRef.Name,
		Id:   ovdcNetwork.Connection.RouterRef.Id,
	}

	// Cache backing type
	gateway.NetworkBackingType = *ovdcNetwork.BackingNetworkType

	//klog.Infof("Obtained GatewayManager [%s] for Network Name [%s] of type [%v]\n",
	//	gateway.GatewayRef.Name, gateway.NetworkName, gateway.NetworkBackingType)

	return nil
}

func getUnusedIPAddressInRange(startIPAddress string, endIPAddress string,
	usedIPAddresses map[string]bool) string {

	// This is not the best approach and can be optimized further by skipping ranges.
	freeIP := ""
	startIP, _, _ := net.ParseCIDR(fmt.Sprintf("%s/32", startIPAddress))
	endIP, _, _ := net.ParseCIDR(fmt.Sprintf("%s/32", endIPAddress))
	for !startIP.Equal(endIP) && usedIPAddresses[startIP.String()] {
		startIP = cidr.Inc(startIP)
	}
	// either the last IP is free or an intermediate IP is not yet used
	if !startIP.Equal(endIP) || startIP.Equal(endIP) && !usedIPAddresses[startIP.String()] {
		freeIP = startIP.String()
	}

	if freeIP != "" {
		klog.Infof("Obtained unused IP [%s] in range [%s-%s]\n", freeIP, startIPAddress, endIPAddress)
	}

	return freeIP
}

func (gateway *GatewayManager) getUnusedInternalIPAddress(ctx context.Context) (string, error) {
	client := gateway.Client
	if client.GatewayRef == nil {
		return "", fmt.Errorf("gateway reference should not be nil")
	}

	usedIPAddress := make(map[string]bool)
	pageNum := int32(1)
	for {
		lbVSSummaries, resp, err := client.ApiClient.EdgeGatewayLoadBalancerVirtualServicesApi.GetVirtualServiceSummariesForGateway(
			ctx, pageNum, 25, client.GatewayRef.Id, nil)
		if err != nil {
			return "", fmt.Errorf("unable to get virtual service summaries for gateway [%s]: resp: [%v]: [%v]",
				client.GatewayRef.Name, resp, err)
		}
		if len(lbVSSummaries.Values) == 0 {
			break
		}
		for _, lbVSSummary := range lbVSSummaries.Values {
			usedIPAddress[lbVSSummary.VirtualIpAddress] = true
		}

		pageNum++
	}

	freeIP := getUnusedIPAddressInRange(client.OneArm.StartIPAddress,
		client.OneArm.EndIPAddress, usedIPAddress)
	if freeIP == "" {
		return "", fmt.Errorf("unable to find unused IP address in range [%s-%s]",
			client.OneArm.StartIPAddress, client.OneArm.EndIPAddress)
	}

	return freeIP, nil
}

// There are races here since there is no 'acquisition' of an IP. However, since k8s retries, it will
// be correct.
func (gateway *GatewayManager) getUnusedExternalIPAddress(ctx context.Context, ipamSubnet string) (string, error) {
	client := gateway.Client
	if client.GatewayRef == nil {
		return "", fmt.Errorf("gateway reference should not be nil")
	}

	// First, get list of ip ranges for the IPAMSubnet subnet mask
	edgeGW, resp, err := client.ApiClient.EdgeGatewayApi.GetEdgeGateway(ctx, client.GatewayRef.Id)
	if err != nil {
		return "", fmt.Errorf("unable to retrieve edge gateway details for [%s]: resp [%+v]: [%v]",
			client.GatewayRef.Name, resp, err)
	}

	var ipRanges *swaggerClient.IpRanges = nil
	for _, edgeGWUplink := range edgeGW.EdgeGatewayUplinks {
		for _, subnet := range edgeGWUplink.Subnets.Values {
			subnetMask := fmt.Sprintf("%s/%d", subnet.Gateway, subnet.PrefixLength)
			if subnetMask == ipamSubnet {
				ipRanges = subnet.IpRanges
				break
			}
		}
	}
	if ipRanges == nil {
		return "", fmt.Errorf("unable to get ipRange corresponding to IPAM subnet mask [%s]",
			ipamSubnet)
	}

	// Next, get the list of used IP addresses for this gateway
	usedIPs := make(map[string]bool)
	pageNum := int32(1)
	for {
		gwUsedIPAddresses, resp, err := client.ApiClient.EdgeGatewayApi.GetUsedIpAddresses(ctx, pageNum, 25,
			client.GatewayRef.Id, nil)
		if err != nil {
			return "", fmt.Errorf("unable to get used IP addresses of gateway [%s]: [%+v]: [%v]",
				client.GatewayRef.Name, resp, err)
		}
		if len(gwUsedIPAddresses.Values) == 0 {
			break
		}

		for _, gwUsedIPAddress := range gwUsedIPAddresses.Values {
			usedIPs[gwUsedIPAddress.IpAddress] = true
		}

		pageNum++
	}

	// Now get a free IP that is not used.
	freeIP := ""
	for _, ipRange := range ipRanges.Values {
		freeIP = getUnusedIPAddressInRange(ipRange.StartAddress,
			ipRange.EndAddress, usedIPs)
		if freeIP != "" {
			break
		}
	}
	if freeIP == "" {
		return "", fmt.Errorf("unable to obtain free IP from gateway [%s]; all are used",
			client.GatewayRef.Name)
	}
	klog.Infof("Using unused IP [%s] on gateway [%v]\n", freeIP, client.GatewayRef.Name)

	return freeIP, nil
}

// TODO: There could be a race here as we don't book a slot. Retry repeatedly to get a LB Segment.
func (gateway *GatewayManager) getLoadBalancerSEG(ctx context.Context) (*swaggerClient.EntityReference, error) {
	client := gateway.Client
	if client.GatewayRef == nil {
		return nil, fmt.Errorf("gateway reference should not be nil")
	}

	pageNum := int32(1)
	var chosenSEGAssignment *swaggerClient.LoadBalancerServiceEngineGroupAssignment = nil
	for {
		segAssignments, resp, err := client.ApiClient.LoadBalancerServiceEngineGroupAssignmentsApi.GetServiceEngineGroupAssignments(
			ctx, pageNum, 25,
			&swaggerClient.LoadBalancerServiceEngineGroupAssignmentsApiGetServiceEngineGroupAssignmentsOpts{
				Filter: optional.NewString(fmt.Sprintf("gatewayRef.id==%s", client.GatewayRef.Id)),
			},
		)
		if err != nil {
			return nil, fmt.Errorf("unable to get service engine group for gateway [%s]: resp: [%v]: [%v]",
				client.GatewayRef.Name, resp, err)
		}
		if len(segAssignments.Values) == 0 {
			return nil, fmt.Errorf("obtained no service engine group assignment for gateway [%s]: [%v]", client.GatewayRef.Name, err)
		}

		for _, segAssignment := range segAssignments.Values {
			if segAssignment.NumDeployedVirtualServices < segAssignment.MaxVirtualServices {
				chosenSEGAssignment = &segAssignment
				break
			}
		}
		if chosenSEGAssignment != nil {
			break
		}

		pageNum++
	}

	if chosenSEGAssignment == nil {
		return nil, fmt.Errorf("unable to find service engine group with free instances")
	}

	klog.Infof("Using service engine group [%v] on gateway [%v]\n", chosenSEGAssignment.ServiceEngineGroupRef, client.GatewayRef.Name)

	return chosenSEGAssignment.ServiceEngineGroupRef, nil
}

func getCursor(resp *http.Response) (string, error) {
	cursorURI := ""
	for _, linklet := range resp.Header["Link"] {
		for _, l := range link.Parse(linklet) {
			if l.Rel == "nextPage" {
				cursorURI = l.URI
				break
			}
		}
		if cursorURI != "" {
			break
		}
	}
	if cursorURI == "" {
		return "", nil
	}

	u, err := url.Parse(cursorURI)
	if err != nil {
		return "", fmt.Errorf("unable to parse cursor URI [%s]: [%v]", cursorURI, err)
	}

	cursorStr := ""
	keyMap, err := url.ParseQuery(u.RawQuery)
	if err != nil {
		return "", fmt.Errorf("unable to parse raw query [%s]: [%v]", u.RawQuery, err)
	}

	if cursorStrList, ok := keyMap["cursor"]; ok {
		cursorStr = cursorStrList[0]
	}

	return cursorStr, nil
}

type NatRuleRef struct {
	Name         string
	ID           string
	ExternalIP   string
	InternalIP   string
	ExternalPort int
	InternalPort int
}

func (gateway *GatewayManager) getNATRuleRef(ctx context.Context,
	natRuleName string) (*NatRuleRef, error) {
	client := gateway.Client

	if client.GatewayRef == nil {
		return nil, fmt.Errorf("gateway reference should not be nil")
	}

	var natRuleRef *NatRuleRef = nil
	cursor := optional.EmptyString()
	for {
		natRules, resp, err := client.ApiClient.EdgeGatewayNatRulesApi.GetNatRules(
			ctx, 128, client.GatewayRef.Id,
			&swaggerClient.EdgeGatewayNatRulesApiGetNatRulesOpts{
				Cursor: cursor,
			})
		if err != nil {
			return nil, fmt.Errorf("unable to get nat rules: resp: [%+v]: [%v]", resp, err)
		}
		if len(natRules.Values) == 0 {
			break
		}

		for _, rule := range natRules.Values {
			if rule.Name == natRuleName {
				externalPort := 0
				if rule.DnatExternalPort != "" {
					externalPort, err = strconv.Atoi(rule.DnatExternalPort)
					if err != nil {
						return nil, fmt.Errorf("Unable to convert external port [%s] to int: [%v]",
							rule.DnatExternalPort, err)
					}
				}

				internalPort := 0
				if rule.InternalPort != "" {
					internalPort, err = strconv.Atoi(rule.InternalPort)
					if err != nil {
						return nil, fmt.Errorf("Unable to convert internal port [%s] to int: [%v]",
							rule.InternalPort, err)
					}
				}

				natRuleRef = &NatRuleRef{
					ID:           rule.Id,
					Name:         rule.Name,
					ExternalIP:   rule.ExternalAddresses,
					InternalIP:   rule.InternalAddresses,
					ExternalPort: externalPort,
					InternalPort: internalPort,
				}
				break
			}
		}
		if natRuleRef != nil {
			break
		}

		cursorStr, err := getCursor(resp)
		if err != nil {
			return nil, fmt.Errorf("error while parsing response [%+v]: [%v]", resp, err)
		}
		if cursorStr == "" {
			break
		}
		cursor = optional.NewString(cursorStr)
	}

	if natRuleRef == nil {
		return nil, nil // this is not an error
	}

	return natRuleRef, nil
}

// TODO: get and return if it already exists
func (gateway *GatewayManager) createDNATRule(ctx context.Context, dnatRuleName string,
	externalIP string, internalIP string, port int32) error {
	client := gateway.Client
	if client.GatewayRef == nil {
		return fmt.Errorf("gateway reference should not be nil")
	}

	dnatRuleRef, err := gateway.getNATRuleRef(ctx, dnatRuleName)
	if err != nil {
		return fmt.Errorf("unexpected error while looking for nat rule [%s] in gateway [%s]: [%v]",
			dnatRuleName, client.GatewayRef.Name, err)
	}
	if dnatRuleRef != nil {
		klog.Infof("DNAT Rule [%s] already exists", dnatRuleName)
		return nil
	}

	ruleType := swaggerClient.NatRuleType(swaggerClient.DNAT_NatRuleType)
	edgeNatRule := swaggerClient.EdgeNatRule{
		Name:              dnatRuleName,
		Enabled:           true,
		RuleType:          &ruleType,
		ExternalAddresses: externalIP,
		InternalAddresses: internalIP,
		DnatExternalPort:  fmt.Sprintf("%d", port),
	}
	resp, err := client.ApiClient.EdgeGatewayNatRulesApi.CreateNatRule(ctx, edgeNatRule, client.GatewayRef.Id)
	if resp != nil && resp.StatusCode != http.StatusAccepted {
		return fmt.Errorf("unable to create dnat rule [%s]: [%s]=>[%s]; expected http response [%v], obtained [%v]: [%v]",
			dnatRuleName, externalIP, internalIP, http.StatusAccepted, resp.StatusCode, err)
	} else if err != nil {
		return fmt.Errorf("unable to create dnat rule [%s]: [%s:%d]=>[%s:%d]: [%v]", dnatRuleName,
			externalIP, port, internalIP, port, err)
	}

	taskURL := resp.Header.Get("Location")
	task := govcd.NewTask(&client.VcdClient.Client)
	task.Task.HREF = taskURL
	if err = task.WaitTaskCompletion(); err != nil {
		return fmt.Errorf("unable to create dnat rule [%s]: [%s]=>[%s]; creation task [%s] did not complete: [%v]",
			dnatRuleName, externalIP, internalIP, taskURL, err)
	}

	klog.Infof("Created DNAT rule [%s]: [%s:%d] => [%s] on gateway [%s]\n", dnatRuleName,
		externalIP, port, internalIP, client.GatewayRef.Name)

	return nil
}

func (gateway *GatewayManager) deleteDNATRule(ctx context.Context, dnatRuleName string,
	failIfAbsent bool) error {
	client := gateway.Client
	if client.GatewayRef == nil {
		return fmt.Errorf("gateway reference should not be nil")
	}

	dnatRuleRef, err := gateway.getNATRuleRef(ctx, dnatRuleName)
	if err != nil {
		return fmt.Errorf("unexpected error while finding dnat rule [%s]: [%v]", dnatRuleName, err)
	}
	if dnatRuleRef == nil {
		if failIfAbsent {
			return fmt.Errorf("dnat rule [%s] does not exist", dnatRuleName)
		}

		return nil
	}

	resp, err := client.ApiClient.EdgeGatewayNatRuleApi.DeleteNatRule(ctx,
		client.GatewayRef.Id, dnatRuleRef.ID)
	if resp.StatusCode != http.StatusAccepted {
		return fmt.Errorf("unable to delete dnat rule [%s]: expected http response [%v], obtained [%v]",
			dnatRuleName, http.StatusAccepted, resp.StatusCode)
	}

	taskURL := resp.Header.Get("Location")
	task := govcd.NewTask(&client.VcdClient.Client)
	task.Task.HREF = taskURL
	if err = task.WaitTaskCompletion(); err != nil {
		return fmt.Errorf("unable to delete dnat rule [%s]: deletion task [%s] did not complete: [%v]",
			dnatRuleName, taskURL, err)
	}
	klog.Infof("Deleted DNAT rule [%s] on gateway [%s]\n", dnatRuleName, client.GatewayRef.Name)

	return nil
}

func (gateway *GatewayManager) GetLoadBalancerPool(ctx context.Context, lbPoolName string) (*swaggerClient.EntityReference, error) {
	lbPoolRef, err := gateway.getLoadBalancerPool(ctx, lbPoolName)
	if err != nil {
		return nil, fmt.Errorf("unable to get reference for LB pool [%s]: [%v]", lbPoolName, err)
	}
	if lbPoolRef == nil {
		return nil, govcd.ErrorEntityNotFound
	}
	return lbPoolRef, nil
}

func (gateway *GatewayManager) GetLoadBalancerPoolMemberIPs(ctx context.Context, lbPoolRef *swaggerClient.EntityReference) ([]string, error) {
	client := gateway.Client
	if lbPoolRef == nil {
		return nil, govcd.ErrorEntityNotFound
	}
	lbPool, resp, err := client.ApiClient.EdgeGatewayLoadBalancerPoolApi.GetLoadBalancerPool(ctx, lbPoolRef.Id)
	if err != nil {
		return nil, fmt.Errorf("unable to get the details for LB pool [%s]: [%+v]: [%v]",
			lbPoolRef.Name, resp, err)
	}

	memberIPs := make([]string, lbPool.MemberCount)
	members := lbPool.Members
	for i, member := range members {
		memberIPs[i] = member.IpAddress
	}
	return memberIPs, nil
}

func (gateway *GatewayManager) getLoadBalancerPoolSummary(ctx context.Context,
	lbPoolName string) (*swaggerClient.EdgeLoadBalancerPoolSummary, error) {
	client := gateway.Client
	if client.GatewayRef == nil {
		return nil, fmt.Errorf("gateway reference should not be nil")
	}

	// This should return exactly one result, so no need to accumulate results
	lbPoolSummaries, resp, err := client.ApiClient.EdgeGatewayLoadBalancerPoolsApi.GetPoolSummariesForGateway(
		ctx, 1, 25, client.GatewayRef.Id,
		&swaggerClient.EdgeGatewayLoadBalancerPoolsApiGetPoolSummariesForGatewayOpts{
			Filter: optional.NewString(fmt.Sprintf("name==%s", lbPoolName)),
		},
	)
	if err != nil {
		return nil, fmt.Errorf("unable to get reference for LB pool [%s]: [%+v]: [%v]",
			lbPoolName, resp, err)
	}
	if len(lbPoolSummaries.Values) != 1 {
		return nil, nil // this is not an error
	}

	return &lbPoolSummaries.Values[0], nil
}

func (gateway *GatewayManager) getLoadBalancerPool(ctx context.Context,
	lbPoolName string) (*swaggerClient.EntityReference, error) {
	lbPoolSummary, err := gateway.getLoadBalancerPoolSummary(ctx, lbPoolName)
	if err != nil {
		return nil, fmt.Errorf("error when getting LB Pool: [%v]", err)
	}
	if lbPoolSummary == nil {
		return nil, nil // this is not an error
	}

	return &swaggerClient.EntityReference{
		Name: lbPoolSummary.Name,
		Id:   lbPoolSummary.Id,
	}, nil
}

func (gateway *GatewayManager) formLoadBalancerPool(lbPoolName string, ips []string,
	internalPort int32) (swaggerClient.EdgeLoadBalancerPool, []swaggerClient.EdgeLoadBalancerPoolMember) {
	client := gateway.Client
	lbPoolMembers := make([]swaggerClient.EdgeLoadBalancerPoolMember, len(ips))
	for i, ip := range ips {
		lbPoolMembers[i].IpAddress = ip
		lbPoolMembers[i].Port = internalPort
		lbPoolMembers[i].Ratio = 1
		lbPoolMembers[i].Enabled = true
	}

	persistenceProfile := &swaggerClient.EdgeLoadBalancerPersistenceProfile{
		Type_: "CLIENT_IP",
	}

	healthMonitors := []swaggerClient.EdgeLoadBalancerHealthMonitor{
		{
			Type_: "TCP",
		},
	}

	gracefulTimeoutPeriod := int32(0)
	lbPool := swaggerClient.EdgeLoadBalancerPool{
		Enabled:               true,
		Name:                  lbPoolName,
		DefaultPort:           internalPort,
		GracefulTimeoutPeriod: &gracefulTimeoutPeriod, // when service outage occurs, immediately mark as bad
		Members:               lbPoolMembers,
		GatewayRef:            client.GatewayRef,
		Algorithm:             "ROUND_ROBIN",
		PersistenceProfile:    persistenceProfile,
		HealthMonitors:        healthMonitors,
	}
	return lbPool, lbPoolMembers
}

func (gateway *GatewayManager) createLoadBalancerPool(ctx context.Context, lbPoolName string,
	ips []string, internalPort int32) (*swaggerClient.EntityReference, error) {
	client := gateway.Client
	if client.GatewayRef == nil {
		return nil, fmt.Errorf("gateway reference should not be nil")
	}

	lbPoolRef, err := gateway.getLoadBalancerPool(ctx, lbPoolName)
	if err != nil {
		return nil, fmt.Errorf("unexpected error when querying for pool [%s]: [%v]",
			lbPoolName, err)
	}
	if lbPoolRef != nil {
		klog.Infof("LoadBalancer Pool [%s] already exists", lbPoolName)
		return lbPoolRef, nil
	}

	lbPool, lbPoolMembers := gateway.formLoadBalancerPool(lbPoolName, ips, internalPort)
	resp, err := client.ApiClient.EdgeGatewayLoadBalancerPoolsApi.CreateLoadBalancerPool(ctx, lbPool)
	if err != nil {
		return nil, fmt.Errorf("unable to create loadbalancer pool with name [%s], members [%+v]: resp [%+v]: [%v]",
			lbPoolName, lbPoolMembers, resp, err)
	}
	if resp.StatusCode != http.StatusAccepted {
		return nil, fmt.Errorf("unable to create loadbalancer pool; expected http response [%v], obtained [%v]",
			http.StatusAccepted, resp.StatusCode)
	}

	taskURL := resp.Header.Get("Location")
	task := govcd.NewTask(&client.VcdClient.Client)
	task.Task.HREF = taskURL
	if err = task.WaitTaskCompletion(); err != nil {
		return nil, fmt.Errorf("unable to create loadbalancer pool; creation task [%s] did not complete: [%v]",
			taskURL, err)
	}

	// Get the pool to return it
	lbPoolRef, err = gateway.getLoadBalancerPool(ctx, lbPoolName)
	if err != nil {
		return nil, fmt.Errorf("unexpected error when querying for pool [%s]: [%v]",
			lbPoolName, err)
	}
	if lbPoolRef == nil {
		return nil, fmt.Errorf("unable to query for loadbalancer pool [%s] that was freshly created: [%v]",
			lbPoolName, err)
	}
	klog.Infof("Created lb pool [%v] on gateway [%v]\n", lbPoolRef, client.GatewayRef.Name)

	return lbPoolRef, nil
}

func (gateway *GatewayManager) deleteLoadBalancerPool(ctx context.Context, lbPoolName string,
	failIfAbsent bool) error {
	client := gateway.Client
	if client.GatewayRef == nil {
		return fmt.Errorf("gateway reference should not be nil")
	}

	lbPoolRef, err := gateway.getLoadBalancerPool(ctx, lbPoolName)
	if err != nil {
		return fmt.Errorf("unexpected error in retrieving loadbalancer pool [%s]: [%v]",
			lbPoolName, err)
	}
	if lbPoolRef == nil {
		if failIfAbsent {
			return fmt.Errorf("LoadBalancer pool [%s] does not exist", lbPoolName)
		}

		return nil
	}

	resp, err := client.ApiClient.EdgeGatewayLoadBalancerPoolApi.DeleteLoadBalancerPool(ctx, lbPoolRef.Id)
	if err != nil {
		return fmt.Errorf("unable to delete lb pool; error calling DeleteLoadBalancerPool: [%v]", err)
	}
	if resp == nil {
		return fmt.Errorf("error deleting load balancer pool; got an empty reponse while deleting load balancer pool: [%v]", err)
	}
	if resp.StatusCode != http.StatusAccepted {
		return fmt.Errorf("unable to delete lb pool; expected http response [%v], obtained [%v]",
			http.StatusAccepted, resp.StatusCode)
	}

	taskURL := resp.Header.Get("Location")
	task := govcd.NewTask(&client.VcdClient.Client)
	task.Task.HREF = taskURL
	if err = task.WaitTaskCompletion(); err != nil {
		return fmt.Errorf("unable to delete lb pool; deletion task [%s] did not complete: [%v]",
			taskURL, err)
	}
	klog.Infof("Deleted loadbalancer pool [%s]\n", lbPoolName)

	return nil
}

func removeDuplicateStrings(arr []string) []string {
	m := make(map[string]bool)
	noDuplicateArr := make([]string, 0)
	for _, str := range arr {
		if _, exists := m[str]; exists == false {
			m[str] = true
			noDuplicateArr = append(noDuplicateArr, str)
		}
	}
	return noDuplicateArr
}

func (gateway *GatewayManager) updateLoadBalancerPool(ctx context.Context, lbPoolName string, ips []string,
	internalPort int32) (*swaggerClient.EntityReference, error) {
	client := gateway.Client
	lbPoolRef, err := gateway.getLoadBalancerPool(ctx, lbPoolName)
	if err != nil {
		return nil, fmt.Errorf("unexpected error when querying for pool [%s]: [%v]", lbPoolName, err)
	}
	if lbPoolRef == nil {
		return nil, fmt.Errorf("no lb pool found with name [%s]: [%v]", lbPoolName, err)
	}

	ips = removeDuplicateStrings(ips)

	lbPool, lbPoolMembers := gateway.formLoadBalancerPool(lbPoolName, ips, internalPort)
	resp, err := client.ApiClient.EdgeGatewayLoadBalancerPoolApi.UpdateLoadBalancerPool(ctx, lbPool, lbPoolRef.Id)
	if err != nil {
		return nil, fmt.Errorf("unable to update loadbalancer pool with name [%s], members [%+v]: resp [%+v]: [%v]",
			lbPoolName, lbPoolMembers, resp, err)
	}
	if resp.StatusCode != http.StatusAccepted {
		return nil, fmt.Errorf("unable to update loadbalancer pool; expected http response [%v], obtained [%v]",
			http.StatusAccepted, resp.StatusCode)
	}

	taskURL := resp.Header.Get("Location")
	task := govcd.NewTask(&client.VcdClient.Client)
	task.Task.HREF = taskURL
	if err = task.WaitTaskCompletion(); err != nil {
		return nil, fmt.Errorf("unable to update loadbalancer pool; update task [%s] did not complete: [%v]",
			taskURL, err)
	}

	// Get the pool to return it
	lbPoolRef, err = gateway.getLoadBalancerPool(ctx, lbPoolName)
	if err != nil {
		return nil, fmt.Errorf("unexpected error when querying for pool [%s]: [%v]",
			lbPoolName, err)
	}
	if lbPoolRef == nil {
		return nil, fmt.Errorf("unable to query for loadbalancer pool [%s] that was updated: [%v]",
			lbPoolName, err)
	}
	klog.Infof("Updated lb pool [%v] on gateway [%v]\n", lbPoolRef, client.GatewayRef.Name)

	return lbPoolRef, nil
}

func (gateway *GatewayManager) getVirtualService(ctx context.Context,
	virtualServiceName string) (*swaggerClient.EdgeLoadBalancerVirtualServiceSummary, error) {
	client := gateway.Client
	if client.GatewayRef == nil {
		return nil, fmt.Errorf("gateway reference should not be nil")
	}

	// This should return exactly one result, so no need to accumulate results
	lbVSSummaries, resp, err := client.ApiClient.EdgeGatewayLoadBalancerVirtualServicesApi.GetVirtualServiceSummariesForGateway(
		ctx, 1, 25, client.GatewayRef.Id,
		&swaggerClient.EdgeGatewayLoadBalancerVirtualServicesApiGetVirtualServiceSummariesForGatewayOpts{
			Filter: optional.NewString(fmt.Sprintf("name==%s", virtualServiceName)),
		},
	)
	if err != nil {
		return nil, fmt.Errorf("unable to get reference for LB VS [%s]: resp: [%v]: [%v]",
			virtualServiceName, resp, err)
	}
	if len(lbVSSummaries.Values) != 1 {
		return nil, nil // this is not an error
	}

	return &lbVSSummaries.Values[0], nil
}

func (gateway *GatewayManager) createVirtualService(ctx context.Context, virtualServiceName string,
	lbPoolRef *swaggerClient.EntityReference, segRef *swaggerClient.EntityReference,
	freeIP string, vsType string, externalPort int32,
	certificateAlias string) (*swaggerClient.EntityReference, error) {
	client := gateway.Client
	if client.GatewayRef == nil {
		return nil, fmt.Errorf("gateway reference should not be nil")
	}

	vsSummary, err := gateway.getVirtualService(ctx, virtualServiceName)
	if err != nil {
		return nil, fmt.Errorf("unexpected error while getting summary for LB VS [%s]: [%v]",
			virtualServiceName, err)
	}
	if vsSummary != nil {
		klog.Infof("LoadBalancer Virtual Service [%s] already exists", virtualServiceName)
		return &swaggerClient.EntityReference{
			Name: vsSummary.Name,
			Id:   vsSummary.Id,
		}, nil
	}

	var virtualServiceConfig *swaggerClient.EdgeLoadBalancerVirtualService = nil
	switch vsType {
	case "HTTP":
		virtualServiceConfig = &swaggerClient.EdgeLoadBalancerVirtualService{
			Name:                  virtualServiceName,
			Enabled:               true,
			VirtualIpAddress:      freeIP,
			LoadBalancerPoolRef:   lbPoolRef,
			GatewayRef:            client.GatewayRef,
			ServiceEngineGroupRef: segRef,
			ServicePorts: []swaggerClient.EdgeLoadBalancerServicePort{
				{
					TcpUdpProfile: &swaggerClient.EdgeLoadBalancerTcpUdpProfile{
						Type_: "TCP_PROXY",
					},
					PortStart:  externalPort,
					SslEnabled: false,
				},
			},
			ApplicationProfile: &swaggerClient.EdgeLoadBalancerApplicationProfile{
				Name:          "System-HTTP",
				Type_:         vsType,
				SystemDefined: true,
			},
		}
		break

	case "HTTPS":
		if certificateAlias == "" {
			return nil, fmt.Errorf("certificate alias should no be empty for HTTPS service")
		}

		certLibItems, resp, err := client.ApiClient.CertificateLibraryApi.QueryCertificateLibrary(ctx,
			1, 128,
			&swaggerClient.CertificateLibraryApiQueryCertificateLibraryOpts{
				Filter: optional.NewString(fmt.Sprintf("alias==%s", certificateAlias)),
			},
		)
		if err != nil {
			return nil, fmt.Errorf("unable to get cert with alias [%s]: resp: [%v]: [%v]",
				certificateAlias, resp, err)
		}
		if len(certLibItems.Values) != 1 {
			return nil, fmt.Errorf("expected 1 cert with alias [%s], obtained [%d]",
				certificateAlias, len(certLibItems.Values))
		}
		virtualServiceConfig = &swaggerClient.EdgeLoadBalancerVirtualService{
			Name:                  virtualServiceName,
			Enabled:               true,
			VirtualIpAddress:      freeIP,
			LoadBalancerPoolRef:   lbPoolRef,
			GatewayRef:            client.GatewayRef,
			ServiceEngineGroupRef: segRef,
			CertificateRef: &swaggerClient.EntityReference{
				Name: certLibItems.Values[0].Alias,
				Id:   certLibItems.Values[0].Id,
			},
			ServicePorts: []swaggerClient.EdgeLoadBalancerServicePort{
				{
					TcpUdpProfile: &swaggerClient.EdgeLoadBalancerTcpUdpProfile{
						Type_: "TCP_PROXY",
					},
					PortStart:  externalPort,
					SslEnabled: true,
				},
			},
			ApplicationProfile: &swaggerClient.EdgeLoadBalancerApplicationProfile{
				Name:          "System-HTTP",
				Type_:         vsType,
				SystemDefined: true,
			},
		}
		break

	case "L4":
		virtualServiceConfig = &swaggerClient.EdgeLoadBalancerVirtualService{
			Name:                  virtualServiceName,
			Enabled:               true,
			VirtualIpAddress:      freeIP,
			LoadBalancerPoolRef:   lbPoolRef,
			GatewayRef:            client.GatewayRef,
			ServiceEngineGroupRef: segRef,
			ServicePorts: []swaggerClient.EdgeLoadBalancerServicePort{
				{
					TcpUdpProfile: &swaggerClient.EdgeLoadBalancerTcpUdpProfile{
						Type_: "TCP_PROXY",
					},
					PortStart:  externalPort,
					SslEnabled: false,
				},
			},
			ApplicationProfile: &swaggerClient.EdgeLoadBalancerApplicationProfile{
				Name:          "System-HTTP",
				Type_:         vsType,
				SystemDefined: true,
			},
		}
		break
	default:
		return nil, fmt.Errorf("unhandled virtual service type [%s]", vsType)
	}

	resp, err := client.ApiClient.EdgeGatewayLoadBalancerVirtualServicesApi.CreateVirtualService(ctx, *virtualServiceConfig)
	if resp != nil && resp.StatusCode != http.StatusAccepted {
		return nil, fmt.Errorf(
			"unable to create virtual service; expected http response [%v], obtained [%v]: resp: [%#v]: [%v]",
			http.StatusAccepted, resp.StatusCode, resp, err)
	} else if err != nil {
		return nil, fmt.Errorf("error while creating virtual service [%s]: [%v]", virtualServiceName, err)
	}

	taskURL := resp.Header.Get("Location")
	task := govcd.NewTask(&client.VcdClient.Client)
	task.Task.HREF = taskURL
	if err = task.WaitTaskCompletion(); err != nil {
		return nil, fmt.Errorf("unable to create virtual service; creation task [%s] did not complete: [%v]",
			taskURL, err)
	}

	vsSummary, err = gateway.getVirtualService(ctx, virtualServiceName)
	if err != nil {
		return nil, fmt.Errorf("unable to get summary for freshly created LB VS [%s]: [%v]",
			virtualServiceName, err)
	}
	if vsSummary == nil {
		return nil, fmt.Errorf("unable to get summary of freshly created virtual service [%s]: [%v]",
			virtualServiceName, err)
	}

	virtualServiceRef := &swaggerClient.EntityReference{
		Name: vsSummary.Name,
		Id:   vsSummary.Id,
	}
	klog.Infof("Created virtual service [%v] on gateway [%v]\n", virtualServiceRef, client.GatewayRef.Name)

	return virtualServiceRef, nil
}

func (gateway *GatewayManager) deleteVirtualService(ctx context.Context, virtualServiceName string,
	failIfAbsent bool) error {
	client := gateway.Client
	if client.GatewayRef == nil {
		return fmt.Errorf("gateway reference should not be nil")
	}

	vsSummary, err := gateway.getVirtualService(ctx, virtualServiceName)
	if err != nil {
		return fmt.Errorf("unable to get summary for LB Virtual Service [%s]: [%v]",
			virtualServiceName, err)
	}
	if vsSummary == nil {
		if failIfAbsent {
			return fmt.Errorf("Virtual Service [%s] does not exist", virtualServiceName)
		}

		return nil
	}

	resp, err := client.ApiClient.EdgeGatewayLoadBalancerVirtualServiceApi.DeleteVirtualService(
		ctx, vsSummary.Id)
	if resp.StatusCode != http.StatusAccepted {
		return fmt.Errorf("unable to delete virtual service [%s]; expected http response [%v], obtained [%v]",
			vsSummary.Name, http.StatusAccepted, resp.StatusCode)
	}

	taskURL := resp.Header.Get("Location")
	task := govcd.NewTask(&client.VcdClient.Client)
	task.Task.HREF = taskURL
	if err = task.WaitTaskCompletion(); err != nil {
		return fmt.Errorf("unable to delete virtual service; deletion task [%s] did not complete: [%v]",
			taskURL, err)
	}
	klog.Infof("Deleted virtual service [%s]\n", virtualServiceName)

	return nil
}

// CreateLoadBalancer : create a new load balancer pool and virtual service pointing to it
func (gateway *GatewayManager) CreateLoadBalancer(ctx context.Context, virtualServiceNamePrefix string,
	lbPoolNamePrefix string, ips []string, httpPort int32, httpsPort int32) (string, error) {
	client := gateway.Client
	client.rwLock.Lock()
	defer client.rwLock.Unlock()

	if httpPort == 0 && httpsPort == 0 {
		// nothing to do here
		klog.Infof("There is no port specified. Hence nothing to do.")
		return "", fmt.Errorf("nothing to do since http and https ports are not specified")
	}

	if client.GatewayRef == nil {
		return "", fmt.Errorf("gateway reference should not be nil")
	}

	type PortDetails struct {
		portSuffix       string
		serviceType      string
		externalPort     int32
		internalPort     int32
		certificateAlias string
	}
	portDetails := []PortDetails{
		{
			portSuffix:       "http",
			serviceType:      "HTTP",
			externalPort:     client.HTTPPort,
			internalPort:     httpPort,
			certificateAlias: "",
		},
		{
			portSuffix:       "https",
			serviceType:      "HTTPS",
			externalPort:     client.HTTPSPort,
			internalPort:     httpsPort,
			certificateAlias: "",
		},
	}

	externalIP, err := gateway.getUnusedExternalIPAddress(ctx, client.IPAMSubnet)
	if err != nil {
		return "", fmt.Errorf("unable to get unused IP address from subnet [%s]: [%v]",
			client.IPAMSubnet, err)
	}
	klog.Infof("Using external IP [%s] for virtual service\n", externalIP)

	for _, portDetail := range portDetails {
		if portDetail.internalPort == 0 {
			klog.Infof("No internal port specified for [%s], hence loadbalancer not created\n", portDetail.portSuffix)
			continue
		}

		virtualServiceName := fmt.Sprintf("%s-%s", virtualServiceNamePrefix, portDetail.portSuffix)
		lbPoolName := fmt.Sprintf("%s-%s", lbPoolNamePrefix, portDetail.portSuffix)

		vsSummary, err := gateway.getVirtualService(ctx, virtualServiceName)
		if err != nil {
			return "", fmt.Errorf("unexpected error while querying for virtual service [%s]: [%v]",
				virtualServiceName, err)
		}
		if vsSummary != nil {
			if vsSummary.LoadBalancerPoolRef.Name != lbPoolName {
				return "", fmt.Errorf("Virtual Service [%s] found with unexpected loadbalancer pool [%s]",
					virtualServiceName, lbPoolName)
			}

			klog.Infof("LoadBalancer Virtual Service [%s] already exists", virtualServiceName)
			continue
		}

		virtualServiceIP := externalIP
		if client.OneArm != nil {
			internalIP, err := gateway.getUnusedInternalIPAddress(ctx)
			if err != nil {
				return "", fmt.Errorf("unable to get internal IP address for one-arm mode: [%v]", err)
			}

			dnatRuleName := fmt.Sprintf("dnat-%s", virtualServiceName)
			if err = gateway.createDNATRule(ctx, dnatRuleName, externalIP, internalIP, portDetail.externalPort); err != nil {
				return "", fmt.Errorf("unable to create dnat rule [%s] => [%s]: [%v]",
					externalIP, internalIP, err)
			}
			// use the internal IP to create virtual service
			virtualServiceIP = internalIP
		}

		segRef, err := gateway.getLoadBalancerSEG(ctx)
		if err != nil {
			return "", fmt.Errorf("unable to get service engine group from edge [%s]: [%v]",
				client.GatewayRef.Name, err)
		}

		lbPoolRef, err := gateway.createLoadBalancerPool(ctx, lbPoolName, ips, portDetail.internalPort)
		if err != nil {
			return "", fmt.Errorf("unable to create load balancer pool [%s]: [%v]", lbPoolName, err)
		}

		virtualServiceRef, err := gateway.createVirtualService(ctx, virtualServiceName, lbPoolRef, segRef,
			virtualServiceIP, portDetail.serviceType, portDetail.externalPort, portDetail.certificateAlias)
		if err != nil {
			return "", fmt.Errorf("unable to create virtual service [%s] with address [%s:%d]: [%v]",
				virtualServiceName, virtualServiceIP, portDetail.externalPort, err)
		}
		klog.Infof("Created Load Balancer with virtual service [%v], pool [%v] on gateway [%s]\n",
			virtualServiceRef, lbPoolRef, client.GatewayRef.Name)
	}

	return externalIP, nil
}

func (gateway *GatewayManager) CreateL4LoadBalancer(ctx context.Context, virtualServiceNamePrefix string,
	lbPoolNamePrefix string, ips []string, tcpPort int32) (string, error) {

	gateway.Client.rwLock.Lock()
	defer gateway.Client.rwLock.Unlock()

	if tcpPort == 0 {
		// nothing to do here
		klog.Infof("There is no tcp port specified. Cannot create L4 load balancer")
		return "", fmt.Errorf("there is no tcp port specified. Cannot create L4 load balancer")
	}

	if gateway.GatewayRef == nil {
		return "", fmt.Errorf("gateway reference should not be nil")
	}

	type PortDetails struct {
		portSuffix       string
		serviceType      string
		externalPort     int32
		internalPort     int32
		certificateAlias string
	}
	portDetails := []PortDetails{
		{
			portSuffix:       "tcp",
			serviceType:      "L4",
			externalPort:     gateway.Client.TCPPort,
			internalPort:     tcpPort,
			certificateAlias: "",
		},
	}

	externalIP, err := gateway.getUnusedExternalIPAddress(ctx, gateway.Client.IPAMSubnet)
	if err != nil {
		return "", fmt.Errorf("unable to get unused IP address from subnet [%s]: [%v]",
			gateway.Client.IPAMSubnet, err)
	}
	klog.Infof("Using external IP [%s] for virtual service\n", externalIP)

	for _, portDetail := range portDetails {
		if portDetail.internalPort == 0 {
			klog.Infof("No internal port specified for [%s], hence loadbalancer not created\n", portDetail.portSuffix)
			continue
		}

		virtualServiceName := fmt.Sprintf("%s-%s", virtualServiceNamePrefix, portDetail.portSuffix)
		lbPoolName := fmt.Sprintf("%s-%s", lbPoolNamePrefix, portDetail.portSuffix)

		vsSummary, err := gateway.getVirtualService(ctx, virtualServiceName)
		if err != nil {
			return "", fmt.Errorf("unexpected error while querying for virtual service [%s]: [%v]",
				virtualServiceName, err)
		}
		if vsSummary != nil {
			if vsSummary.LoadBalancerPoolRef.Name != lbPoolName {
				return "", fmt.Errorf("virtual service [%s] found with unexpected loadbalancer pool [%s]",
					virtualServiceName, lbPoolName)
			}

			klog.Infof("LoadBalancer Virtual Service [%s] already exists", virtualServiceName)
			continue
		}

		virtualServiceIP := externalIP
		if gateway.Client.OneArm != nil {
			internalIP, err := gateway.getUnusedInternalIPAddress(ctx)
			if err != nil {
				return "", fmt.Errorf("unable to get internal IP address for one-arm mode: [%v]", err)
			}

			dnatRuleName := fmt.Sprintf("dnat-%s", virtualServiceName)
			if err = gateway.createDNATRule(ctx, dnatRuleName, externalIP, internalIP, portDetail.externalPort); err != nil {
				return "", fmt.Errorf("unable to create dnat rule [%s] => [%s]: [%v]",
					externalIP, internalIP, err)
			}
			// use the internal IP to create virtual service
			virtualServiceIP = internalIP
		}

		segRef, err := gateway.getLoadBalancerSEG(ctx)
		if err != nil {
			return "", fmt.Errorf("unable to get service engine group from edge [%s]: [%v]",
				gateway.Client.GatewayRef.Name, err)
		}

		lbPoolRef, err := gateway.createLoadBalancerPool(ctx, lbPoolName, ips, portDetail.internalPort)
		if err != nil {
			return "", fmt.Errorf("unable to create load balancer pool [%s]: [%v]", lbPoolName, err)
		}

		virtualServiceRef, err := gateway.createVirtualService(ctx, virtualServiceName, lbPoolRef, segRef,
			virtualServiceIP, portDetail.serviceType, portDetail.externalPort, portDetail.certificateAlias)
		if err != nil {
			return "", fmt.Errorf("unable to create virtual service [%s] with address [%s:%d]: [%v]",
				virtualServiceName, virtualServiceIP, portDetail.externalPort, err)
		}
		klog.Infof("Created Load Balancer with virtual service [%v], pool [%v] on gateway [%s]\n",
			virtualServiceRef, lbPoolRef, gateway.GatewayRef.Name)
	}

	return externalIP, nil
}

func (gateway *GatewayManager) UpdateLoadBalancer(ctx context.Context, lbPoolName string,
	ips []string, internalPort int32) error {
	client := gateway.Client
	client.rwLock.Lock()
	defer client.rwLock.Unlock()

	_, err := gateway.updateLoadBalancerPool(ctx, lbPoolName, ips, internalPort)
	if err != nil {
		return fmt.Errorf("unable to update load balancer pool [%s]: [%v]", lbPoolName, err)
	}

	return nil
}

// DeleteLoadBalancer : create a new load balancer pool and virtual service pointing to it
func (gateway *GatewayManager) DeleteLoadBalancer(ctx context.Context, virtualServiceNamePrefix string,
	lbPoolNamePrefix string) error {
	client := gateway.Client
	client.rwLock.Lock()
	defer client.rwLock.Unlock()

	// TODO: try to continue in case of errors
	var err error

	for _, suffix := range []string{"http", "https", "tcp"} {

		virtualServiceName := fmt.Sprintf("%s-%s", virtualServiceNamePrefix, suffix)
		lbPoolName := fmt.Sprintf("%s-%s", lbPoolNamePrefix, suffix)

		err = gateway.deleteVirtualService(ctx, virtualServiceName, false)
		if err != nil {
			return fmt.Errorf("unable to delete virtual service [%s]: [%v]", virtualServiceName, err)
		}

		err = gateway.deleteLoadBalancerPool(ctx, lbPoolName, false)
		if err != nil {
			return fmt.Errorf("unable to delete load balancer pool [%s]: [%v]", lbPoolName, err)
		}

		if client.OneArm != nil {
			dnatRuleName := fmt.Sprintf("dnat-%s", virtualServiceName)
			err = gateway.deleteDNATRule(ctx, dnatRuleName, false)
			if err != nil {
				return fmt.Errorf("unable to delete dnat rule [%s]: [%v]", dnatRuleName, err)
			}
		}
	}

	return nil
}

// GetLoadBalancer :
func (gateway *GatewayManager) GetLoadBalancer(ctx context.Context, virtualServiceName string) (string, error) {
	client := gateway.Client
	vsSummary, err := gateway.getVirtualService(ctx, virtualServiceName)
	if err != nil {
		return "", fmt.Errorf("unable to get summary for LB Virtual Service [%s]: [%v]",
			virtualServiceName, err)
	}
	if vsSummary == nil {
		return "", fmt.Errorf("unable to get summary for LB Virtual Service [%s]", virtualServiceName)
	}

	vip := vsSummary.VirtualIpAddress
	if client.OneArm != nil {
		dnatRuleName := fmt.Sprintf("dnat-%s", virtualServiceName)
		dnatRuleRef, err := gateway.getNATRuleRef(ctx, dnatRuleName)
		if err != nil {
			return "", fmt.Errorf("Unable to find dnat rule [%s] for virtual service [%s]: [%v]",
				dnatRuleName, virtualServiceName, err)
		}
		if dnatRuleRef == nil {
			return "", fmt.Errorf("dnat rule [%s] for virtual service [%s] not found", dnatRuleName, virtualServiceName)
		}
		vip = dnatRuleRef.ExternalIP
	}

	return vip, nil
}

// IsNSXTBackedGateway : return true if gateway is backed by NSX-T
func (gateway *GatewayManager) IsNSXTBackedGateway() bool {
	client := gateway.Client
	isNSXTBackedGateway :=
		(client.NetworkBackingType == swaggerClient.NSXT_FIXED_SEGMENT_BackingNetworkType) ||
			(client.NetworkBackingType == swaggerClient.NSXT_FLEXIBLE_SEGMENT_BackingNetworkType)

	return isNSXTBackedGateway
}
