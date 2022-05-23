/*
   Copyright 2021 VMware, Inc.
   SPDX-License-Identifier: Apache-2.0
*/

// TODO: log statements should change to klog.V(3)

package vcdsdk

import (
	"context"
	"fmt"
	"github.com/antihax/optional"
	"github.com/apparentlymart/go-cidr/cidr"
	"github.com/peterhellberg/link"
	swaggerClient "github.com/vmware/cloud-provider-for-cloud-director/pkg/vcdswaggerclient"
	"github.com/vmware/go-vcloud-director/v2/govcd"
	"github.com/vmware/go-vcloud-director/v2/types/v56"
	"k8s.io/klog"
	"net"
	"net/http"
	"net/url"
	"strconv"
	"strings"
)

type OneArm struct {
	StartIP string
	EndIP   string
}

type GatewayManager struct {
	NetworkName        string
	GatewayRef         *swaggerClient.EntityReference
	NetworkBackingType swaggerClient.BackingNetworkType
	// Client will be refreshed before each call
	Client     *Client
	IPAMSubnet string
}

// CacheGatewayDetails get gateway reference and cache some details in client object
func (gatewayManager *GatewayManager) cacheGatewayDetails(ctx context.Context) error {

	if gatewayManager.NetworkName == "" {
		return fmt.Errorf("network name should not be empty")
	}

	ovdcNetwork, err := gatewayManager.getOVDCNetwork(ctx, gatewayManager.NetworkName)
	if err != nil {
		return fmt.Errorf("unable to get OVDC network [%s]: [%v]", gatewayManager.NetworkName, err)
	}

	// Cache backing type
	if ovdcNetwork.BackingNetworkType != nil {
		gatewayManager.NetworkBackingType = *ovdcNetwork.BackingNetworkType
	}

	// Cache gateway reference
	if ovdcNetwork.Connection == nil ||
		ovdcNetwork.Connection.RouterRef == nil {
		klog.Infof("Gateway for Network Name [%s] is of type [%v]\n",
			gatewayManager.NetworkName, gatewayManager.NetworkBackingType)
		return nil
	}

	gatewayManager.GatewayRef = &swaggerClient.EntityReference{
		Name: ovdcNetwork.Connection.RouterRef.Name,
		Id:   ovdcNetwork.Connection.RouterRef.Id,
	}

	klog.Infof("Obtained Gateway [%s] for Network Name [%s] of type [%v]\n",
		gatewayManager.GatewayRef.Name, gatewayManager.NetworkName, gatewayManager.NetworkBackingType)

	return nil
}

func NewGatewayManager(ctx context.Context, client *Client, networkName string, ipamSubnet string) (*GatewayManager, error) {
	if networkName == "" {
		return nil, fmt.Errorf("empty network name specified while creating GatewayManger")
	}

	gateway := GatewayManager{
		Client:      client,
		NetworkName: networkName,
		IPAMSubnet:  ipamSubnet,
	}

	err := gateway.cacheGatewayDetails(ctx)
	if err != nil {
		return nil, fmt.Errorf("error caching gateway related details: [%v]", err)
	}
	return &gateway, nil
}

func (gatewayManager *GatewayManager) getOVDCNetwork(ctx context.Context, networkName string) (*swaggerClient.VdcNetwork, error) {
	if networkName == "" {
		return nil, fmt.Errorf("network name should not be empty")
	}

	client := gatewayManager.Client
	ovdcNetworksAPI := client.APIClient.OrgVdcNetworksApi
	pageNum := int32(1)
	ovdcNetworkID := ""
	for {
		ovdcNetworks, resp, err := ovdcNetworksAPI.GetAllVdcNetworks(ctx, pageNum, 32, nil)
		if err != nil {
			// TODO: log resp in debug mode only
			return nil, fmt.Errorf("unable to get all ovdc networks: [%+v]: [%v]", resp, err)
		}

		if len(ovdcNetworks.Values) == 0 {
			break
		}

		for _, ovdcNetwork := range ovdcNetworks.Values {
			if ovdcNetwork.Name == gatewayManager.NetworkName {
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
		return nil, fmt.Errorf("unable to obtain ID for ovdc network name [%s]",
			gatewayManager.NetworkName)
	}

	ovdcNetworkAPI := client.APIClient.OrgVdcNetworkApi
	ovdcNetwork, resp, err := ovdcNetworkAPI.GetOrgVdcNetwork(ctx, ovdcNetworkID)
	if err != nil {
		return nil, fmt.Errorf("unable to get network for id [%s]: [%+v]: [%v]", ovdcNetworkID, resp, err)
	}

	return &ovdcNetwork, nil
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

func (gatewayManager *GatewayManager) GetUnusedInternalIPAddress(ctx context.Context, oneArm *OneArm) (string, error) {

	if oneArm == nil {
		return "", fmt.Errorf("unable to get unused internal IP address as oneArm is nil")
	}
	client := gatewayManager.Client
	if gatewayManager.GatewayRef == nil {
		return "", fmt.Errorf("gateway reference should not be nil")
	}

	usedIPAddress := make(map[string]bool)
	pageNum := int32(1)
	for {
		lbVSSummaries, resp, err := client.APIClient.EdgeGatewayLoadBalancerVirtualServicesApi.GetVirtualServiceSummariesForGateway(
			ctx, pageNum, 25, gatewayManager.GatewayRef.Id, nil)
		if err != nil {
			return "", fmt.Errorf("unable to get virtual service summaries for gateway [%s]: resp: [%v]: [%v]",
				gatewayManager.GatewayRef.Name, resp, err)
		}
		if len(lbVSSummaries.Values) == 0 {
			break
		}
		for _, lbVSSummary := range lbVSSummaries.Values {
			usedIPAddress[lbVSSummary.VirtualIpAddress] = true
		}

		pageNum++
	}

	freeIP := getUnusedIPAddressInRange(oneArm.StartIP, oneArm.EndIP, usedIPAddress)
	if freeIP == "" {
		return "", fmt.Errorf("unable to find unused IP address in range [%s-%s]",
			oneArm.StartIP, oneArm.EndIP)
	}

	return freeIP, nil
}

// There are races here since there is no 'acquisition' of an IP. However, since k8s retries, it will
// be correct.
func (gatewayManager *GatewayManager) GetUnusedExternalIPAddress(ctx context.Context, ipamSubnet string) (string, error) {
	client := gatewayManager.Client
	if gatewayManager.GatewayRef == nil {
		return "", fmt.Errorf("gateway reference should not be nil")
	}

	// First, get list of ip ranges for the IPAMSubnet subnet mask
	edgeGW, resp, err := client.APIClient.EdgeGatewayApi.GetEdgeGateway(ctx, gatewayManager.GatewayRef.Id)
	if err != nil {
		return "", fmt.Errorf("unable to retrieve edge gateway details for [%s]: resp [%+v]: [%v]",
			gatewayManager.GatewayRef.Name, resp, err)
	}

	ipRangesList := make([]*swaggerClient.IpRanges, 0)
	for _, edgeGWUplink := range edgeGW.EdgeGatewayUplinks {
		for _, subnet := range edgeGWUplink.Subnets.Values {
			subnetMask := fmt.Sprintf("%s/%d", subnet.Gateway, subnet.PrefixLength)
			// if there is no specified subnet, look at all ranges
			if ipamSubnet == "" {
				ipRangesList = append(ipRangesList, subnet.IpRanges)
			} else if subnetMask == ipamSubnet {
				ipRangesList = append(ipRangesList, subnet.IpRanges)
				break
			}
		}
	}
	if len(ipRangesList) == 0 {
		return "", fmt.Errorf(
			"unable to get appropriate ipRange corresponding to IPAM subnet mask [%s]",
			ipamSubnet)
	}

	// Next, get the list of used IP addresses for this gateway
	usedIPs := make(map[string]bool)
	pageNum := int32(1)
	for {
		gwUsedIPAddresses, resp, err := client.APIClient.EdgeGatewayApi.GetUsedIpAddresses(ctx, pageNum, 25,
			gatewayManager.GatewayRef.Id, nil)
		if err != nil {
			return "", fmt.Errorf("unable to get used IP addresses of gateway [%s]: [%+v]: [%v]",
				gatewayManager.GatewayRef.Name, resp, err)
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
	for _, ipRanges := range ipRangesList {
		for _, ipRange := range ipRanges.Values {
			freeIP = getUnusedIPAddressInRange(ipRange.StartAddress,
				ipRange.EndAddress, usedIPs)
			if freeIP != "" {
				break
			}
		}
	}
	if freeIP == "" {
		return "", fmt.Errorf("unable to obtain free IP from gateway [%s]; all are used",
			gatewayManager.GatewayRef.Name)
	}
	klog.Infof("Using unused IP [%s] on gateway [%v]\n", freeIP, gatewayManager.GatewayRef.Name)

	return freeIP, nil
}

// TODO: There could be a race here as we don't book a slot. Retry repeatedly to get a LB Segment.
func (gatewayManager *GatewayManager) GetLoadBalancerSEG(ctx context.Context) (*swaggerClient.EntityReference, error) {
	if gatewayManager.GatewayRef == nil {
		return nil, fmt.Errorf("gateway reference should not be nil")
	}

	client := gatewayManager.Client
	pageNum := int32(1)
	var chosenSEGAssignment *swaggerClient.LoadBalancerServiceEngineGroupAssignment = nil
	for {
		segAssignments, resp, err := client.APIClient.LoadBalancerServiceEngineGroupAssignmentsApi.GetServiceEngineGroupAssignments(
			ctx, pageNum, 25,
			&swaggerClient.LoadBalancerServiceEngineGroupAssignmentsApiGetServiceEngineGroupAssignmentsOpts{
				Filter: optional.NewString(fmt.Sprintf("gatewayRef.id==%s", gatewayManager.GatewayRef.Id)),
			},
		)
		if err != nil {
			return nil, fmt.Errorf("unable to get service engine group for gateway [%s]: resp: [%v]: [%v]",
				gatewayManager.GatewayRef.Name, resp, err)
		}
		if len(segAssignments.Values) == 0 {
			return nil, fmt.Errorf("obtained no service engine group assignment for gateway [%s]: [%v]", gatewayManager.GatewayRef.Name, err)
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

	klog.Infof("Using service engine group [%v] on gateway [%v]\n", chosenSEGAssignment.ServiceEngineGroupRef, gatewayManager.GatewayRef.Name)

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

// GetNATRuleRef: returns nil if the rule is not found;
func (gatewayManager *GatewayManager) GetNATRuleRef(ctx context.Context, natRuleName string) (*NatRuleRef, error) {

	if gatewayManager.GatewayRef == nil {
		return nil, fmt.Errorf("gateway reference should not be nil")
	}
	client := gatewayManager.Client
	var natRuleRef *NatRuleRef = nil
	cursor := optional.EmptyString()
	for {
		natRules, resp, err := client.APIClient.EdgeGatewayNatRulesApi.GetNatRules(
			ctx, 128, gatewayManager.GatewayRef.Id,
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
						return nil, fmt.Errorf("unable to convert external port [%s] to int: [%v]",
							rule.DnatExternalPort, err)
					}
				}

				internalPort := 0
				if rule.InternalPort != "" {
					internalPort, err = strconv.Atoi(rule.InternalPort)
					if err != nil {
						return nil, fmt.Errorf("unable to convert internal port [%s] to int: [%v]",
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

func GetDNATRuleName(virtualServiceName string) string {
	return fmt.Sprintf("dnat-%s", virtualServiceName)
}

func GetAppPortProfileName(dnatRuleName string) string {
	return fmt.Sprintf("appPort_%s", dnatRuleName)
}

func (gatewayManager *GatewayManager) CreateAppPortProfile(appPortProfileName string, externalPort int32) (*govcd.NsxtAppPortProfile, error) {
	client := gatewayManager.Client
	org, err := client.VCDClient.GetOrgByName(client.ClusterOrgName)
	if err != nil {
		return nil, fmt.Errorf("unable to find org [%s] by name: [%v]", client.ClusterOrgName, err)
	}

	klog.Infof("Verifying if app port profile [%s] exists in org [%s]...", appPortProfileName,
		client.ClusterOrgName)
	// we always use tenant scoped profiles
	contextEntityID := client.VDC.Vdc.ID
	scope := types.ApplicationPortProfileScopeTenant
	appPortProfile, err := org.GetNsxtAppPortProfileByName(appPortProfileName, scope)
	if err != nil && !strings.Contains(err.Error(), govcd.ErrorEntityNotFound.Error()) {
		return nil, fmt.Errorf("unable to search for Application Port Profile [%s]: [%v]",
			appPortProfileName, err)
	}
	if appPortProfile != nil {
		return appPortProfile, nil
	}
	if err == nil {
		// this should not arise
		return nil, fmt.Errorf("obtained empty Application Port Profile even though there was no error")
	}

	klog.Infof("App Port Profile [%s] in org [%s] does not exist.", appPortProfileName,
		client.ClusterOrgName)

	appPortProfileConfig := &types.NsxtAppPortProfile{
		Name:        appPortProfileName,
		Description: fmt.Sprintf("App Port Profile [%s]", appPortProfileName),
		ApplicationPorts: []types.NsxtAppPortProfilePort{
			{
				Protocol: "TCP",
				// We use the externalPort itself, since the LB does the ExternalPort=>InternalPort
				// translation.
				DestinationPorts: []string{fmt.Sprintf("%d", externalPort)},
			},
		},
		OrgRef: &types.OpenApiReference{
			Name: org.Org.Name,
			ID:   org.Org.ID,
		},
		ContextEntityId: contextEntityID,
		Scope:           scope,
	}

	klog.Infof("Creating App Port Profile [%s] in org [%s]...", appPortProfileName,
		client.ClusterOrgName)
	appPortProfile, err = org.CreateNsxtAppPortProfile(appPortProfileConfig)
	if err != nil {
		return nil, fmt.Errorf("unable to create nsxt app port profile with config [%#v]: [%v]",
			appPortProfileConfig, err)
	}

	klog.Infof("Created App Port Profile [%s] in org [%s].", appPortProfileName,
		client.ClusterOrgName)
	return appPortProfile, nil
}

func (gatewayManager *GatewayManager) CreateDNATRule(ctx context.Context, dnatRuleName string,
	externalIP string, internalIP string, externalPort int32, internalPort int32, appPortProfile *govcd.NsxtAppPortProfile) error {

	if gatewayManager.GatewayRef == nil {
		return fmt.Errorf("gateway reference should not be nil")
	}

	client := gatewayManager.Client
	dnatRuleRef, err := gatewayManager.GetNATRuleRef(ctx, dnatRuleName)
	if err != nil {
		return fmt.Errorf("unexpected error while looking for nat rule [%s] in gateway [%s]: [%v]",
			dnatRuleName, gatewayManager.GatewayRef.Name, err)
	}
	if dnatRuleRef != nil {
		klog.Infof("DNAT Rule [%s] already exists", dnatRuleName)
		return nil
	}

	if appPortProfile == nil || appPortProfile.NsxtAppPortProfile == nil {
		return fmt.Errorf("empty app port profile")
	}

	ruleType := swaggerClient.DNAT_NatRuleType
	edgeNatRule := swaggerClient.EdgeNatRule{
		Name:              dnatRuleName,
		Enabled:           true,
		RuleType:          &ruleType,
		ExternalAddresses: externalIP,
		InternalAddresses: internalIP,
		DnatExternalPort:  fmt.Sprintf("%d", externalPort),
		ApplicationPortProfile: &swaggerClient.EntityReference{
			Name: appPortProfile.NsxtAppPortProfile.Name,
			Id:   appPortProfile.NsxtAppPortProfile.ID,
		},
	}
	resp, err := client.APIClient.EdgeGatewayNatRulesApi.CreateNatRule(ctx, edgeNatRule, gatewayManager.GatewayRef.Id)
	if err != nil {
		return fmt.Errorf("unable to create dnat rule [%s]: [%s:%d]=>[%s:%d]: [%v]", dnatRuleName,
			externalIP, externalPort, internalIP, internalPort, err)
	}
	if resp != nil && resp.StatusCode != http.StatusAccepted {
		return fmt.Errorf(
			"unable to create dnat rule [%s]: [%s]=>[%s]; expected http response [%v], obtained [%v]: [%v]",
			dnatRuleName, externalIP, internalIP, http.StatusAccepted, resp.StatusCode, err)
	} else if err != nil {
		return fmt.Errorf("unable to create dnat rule [%s]: [%s:%d]=>[%s:%d]: [%v]", dnatRuleName,
			externalIP, externalPort, internalIP, internalPort, err)
	}

	taskURL := resp.Header.Get("Location")
	task := govcd.NewTask(&client.VCDClient.Client)
	task.Task.HREF = taskURL
	if err = task.WaitTaskCompletion(); err != nil {
		return fmt.Errorf("unable to create dnat rule [%s]: [%s]=>[%s]; creation task [%s] did not complete: [%v]",
			dnatRuleName, externalIP, internalIP, taskURL, err)
	}

	dnatRuleRef, err = gatewayManager.GetNATRuleRef(ctx, dnatRuleName)
	if err != nil {
		return fmt.Errorf("unexpected error while looking for nat rule [%s] after creating it in gateway [%s]: [%v]",
			dnatRuleName, gatewayManager.GatewayRef.Name, err)
	}
	if dnatRuleRef == nil {
		return fmt.Errorf("could not find the created DNAT rule [%s] in gateway: [%v]", dnatRuleName, err)
	}

	klog.Infof("Created DNAT rule [%s]: [%s:%d] => [%s:%d] on gateway [%s]\n", dnatRuleName,
		externalIP, externalPort, internalIP, internalPort, gatewayManager.GatewayRef.Name)

	return nil
}

func (gatewayManager *GatewayManager) UpdateAppPortProfile(appPortProfileName string, externalPort int32) (*govcd.NsxtAppPortProfile, error) {
	client := gatewayManager.Client
	org, err := client.VCDClient.GetOrgByName(client.ClusterOrgName)
	if err != nil {
		return nil, fmt.Errorf("unable to find org [%s] by name: [%v]", client.ClusterOrgName, err)
	}
	appPortProfile, err := org.GetNsxtAppPortProfileByName(appPortProfileName, types.ApplicationPortProfileScopeTenant)
	if err != nil {
		return nil, fmt.Errorf("failed to get application port profile by name [%s]: [%v]", appPortProfileName, err)
	}
	if appPortProfile == nil || appPortProfile.NsxtAppPortProfile == nil || len(appPortProfile.NsxtAppPortProfile.ApplicationPorts) == 0 || len(appPortProfile.NsxtAppPortProfile.ApplicationPorts[0].DestinationPorts) == 0 {
		return nil, fmt.Errorf("invalid app port profile [%s]", appPortProfileName)
	}

	if appPortProfile.NsxtAppPortProfile.ApplicationPorts[0].DestinationPorts[0] == fmt.Sprintf("%d", externalPort) {
		klog.Infof("Update to application port profile [%s] is not required", appPortProfileName)
		return appPortProfile, nil
	}
	appPortProfile.NsxtAppPortProfile.ApplicationPorts[0].DestinationPorts[0] = fmt.Sprintf("%d", externalPort)
	appPortProfile, err = appPortProfile.Update(appPortProfile.NsxtAppPortProfile)
	if err != nil {
		return nil, fmt.Errorf("failed to update application port profile")
	}
	klog.Infof("successfully updated app port profile [%s]", appPortProfileName)
	return appPortProfile, nil
}

func (gatewayManager *GatewayManager) UpdateDNATRule(ctx context.Context, dnatRuleName string, externalIP string, internalIP string, externalPort int32) (*NatRuleRef, error) {
	client := gatewayManager.Client
	if err := gatewayManager.checkIfGatewayIsReady(ctx); err != nil {
		klog.Errorf("failed to update DNAT rule; gateway [%s] is busy", gatewayManager.GatewayRef.Name)
		return nil, err
	}
	dnatRuleRef, err := gatewayManager.GetNATRuleRef(ctx, dnatRuleName)
	if err != nil {
		return nil, fmt.Errorf("unexpected error while looking for nat rule [%s] in gateway [%s]: [%v]",
			dnatRuleName, gatewayManager.GatewayRef.Name, err)
	}
	if dnatRuleRef == nil {
		return nil, fmt.Errorf("failed to get DNAT rule name [%s]", dnatRuleName)
	}
	dnatRule, resp, err := client.APIClient.EdgeGatewayNatRuleApi.GetNatRule(ctx, gatewayManager.GatewayRef.Id, dnatRuleRef.ID)
	if resp != nil && resp.StatusCode != http.StatusOK {
		var responseMessageBytes []byte
		if gsErr, ok := err.(swaggerClient.GenericSwaggerError); ok {
			responseMessageBytes = gsErr.Body()
		}
		return nil, fmt.Errorf(
			"unable to get DNAT rule [%s]; expected http response [%v], obtained [%v]: resp: [%#v]: [%v]",
			dnatRuleRef.Name, http.StatusOK, resp.StatusCode, string(responseMessageBytes), err)
	} else if err != nil {
		return nil, fmt.Errorf("error while getting DNAT rule [%s]: [%v]", dnatRuleRef.Name, err)
	}

	if dnatRule.ExternalAddresses == externalIP &&
		dnatRule.InternalAddresses == internalIP && dnatRule.DnatExternalPort == strconv.FormatInt(int64(externalPort), 10) {
		klog.Infof("Update to DNAT rule [%s] not required", dnatRuleRef.Name)
		return dnatRuleRef, nil
	}
	// update DNAT rule
	dnatRule.ExternalAddresses = externalIP
	dnatRule.InternalAddresses = internalIP
	dnatRule.DnatExternalPort = fmt.Sprintf("%d", externalPort)
	resp, err = client.APIClient.EdgeGatewayNatRuleApi.UpdateNatRule(ctx, dnatRule, gatewayManager.GatewayRef.Id, dnatRuleRef.ID)
	if resp != nil && resp.StatusCode != http.StatusAccepted {
		var responseMessageBytes []byte
		if gsErr, ok := err.(swaggerClient.GenericSwaggerError); ok {
			responseMessageBytes = gsErr.Body()
		}
		return nil, fmt.Errorf(
			"unable to update DNAT rule [%s]; expected http response [%v], obtained [%v]: resp: [%#v]: [%v]",
			dnatRuleRef.Name, http.StatusAccepted, resp.StatusCode, string(responseMessageBytes), err)
	} else if err != nil {
		return nil, fmt.Errorf("error while updating DNAT rule [%s]: [%v]", dnatRuleRef.Name, err)
	}
	taskURL := resp.Header.Get("Location")
	task := govcd.NewTask(&client.VCDClient.Client)
	task.Task.HREF = taskURL
	if err = task.WaitTaskCompletion(); err != nil {
		return nil, fmt.Errorf("unable to delete dnat rule [%s]: deletion task [%s] did not complete: [%v]",
			dnatRuleName, taskURL, err)
	}

	dnatRuleRef, err = gatewayManager.GetNATRuleRef(ctx, dnatRuleName)
	if err != nil {
		return nil, fmt.Errorf("unexpected error while looking for nat rule [%s] in gateway [%s]: [%v]",
			dnatRuleName, gatewayManager.GatewayRef.Name, err)
	}
	if dnatRuleRef == nil {
		return nil, fmt.Errorf("failed to get DNAT rule name [%s]", dnatRuleName)
	}

	klog.Infof("successfully updated DNAT rule [%s] on gateway [%s]", dnatRuleRef.Name, gatewayManager.GatewayRef.Name)
	return dnatRuleRef, nil
}

func (gatewayManager *GatewayManager) DeleteAppPortProfile(appPortProfileName string, failIfAbsent bool) error {
	client := gatewayManager.Client
	klog.Infof("Checking if App Port Profile [%s] in org [%s] exists", appPortProfileName,
		client.ClusterOrgName)

	org, err := client.VCDClient.GetOrgByName(client.ClusterOrgName)
	if err != nil {
		return fmt.Errorf("unable to find org [%s] by name: [%v]", client.ClusterOrgName, err)
	}

	// we always use tenant scoped profiles
	scope := types.ApplicationPortProfileScopeTenant
	appPortProfile, err := org.GetNsxtAppPortProfileByName(appPortProfileName, scope)
	if err != nil {
		if strings.Contains(err.Error(), govcd.ErrorEntityNotFound.Error()) {
			// things to delete are done
			return nil
		}

		return fmt.Errorf("unable to search for Application Port Profile [%s]: [%v]",
			appPortProfileName, err)
	}

	if appPortProfile == nil {
		if failIfAbsent {
			return fmt.Errorf("app port profile [%s] does not exist in org [%s]",
				appPortProfileName, client.ClusterOrgName)
		}

		klog.Infof("App Port Profile [%s] does not exist", appPortProfileName)
	} else {
		klog.Infof("Deleting App Port Profile [%s] in org [%s]", appPortProfileName,
			client.ClusterOrgName)
		if err = appPortProfile.Delete(); err != nil {
			return fmt.Errorf("unable to delete application port profile [%s]: [%v]", appPortProfileName, err)
		}
	}
	return nil
}

// Note that this also deletes App Port Profile Config. So we always need to call this
// even if we don't find a DNAT rule, to ensure that everything is cleaned up.
func (gatewayManager *GatewayManager) DeleteDNATRule(ctx context.Context, dnatRuleName string,
	failIfAbsent bool) error {

	client := gatewayManager.Client
	if err := gatewayManager.checkIfGatewayIsReady(ctx); err != nil {
		klog.Errorf("failed to update DNAT rule; gateway [%s] is busy", gatewayManager.GatewayRef.Name)
		return err
	}

	if gatewayManager.GatewayRef == nil {
		return fmt.Errorf("gateway reference should not be nil")
	}

	dnatRuleRef, err := gatewayManager.GetNATRuleRef(ctx, dnatRuleName)
	if err != nil {
		return fmt.Errorf("unexpected error while finding dnat rule [%s]: [%v]", dnatRuleName, err)
	}
	if dnatRuleRef == nil {
		if failIfAbsent {
			return fmt.Errorf("dnat rule [%s] does not exist", dnatRuleName)
		}

		klog.Infof("DNAT rule [%s] does not exist", dnatRuleName)
	} else {
		resp, err := client.APIClient.EdgeGatewayNatRuleApi.DeleteNatRule(ctx,
			gatewayManager.GatewayRef.Id, dnatRuleRef.ID)
		if resp.StatusCode != http.StatusAccepted {
			var responseMessageBytes []byte
			if gsErr, ok := err.(swaggerClient.GenericSwaggerError); ok {
				responseMessageBytes = gsErr.Body()
			}
			return fmt.Errorf("unable to delete dnat rule [%s]: expected http response [%v], obtained [%v], response: [%v]",
				dnatRuleName, http.StatusAccepted, resp.StatusCode, string(responseMessageBytes))
		}

		taskURL := resp.Header.Get("Location")
		task := govcd.NewTask(&client.VCDClient.Client)
		task.Task.HREF = taskURL
		if err = task.WaitTaskCompletion(); err != nil {
			return fmt.Errorf("unable to delete dnat rule [%s]: deletion task [%s] did not complete: [%v]",
				dnatRuleName, taskURL, err)
		}
		klog.Infof("Deleted DNAT rule [%s] on gateway [%s]\n", dnatRuleName, gatewayManager.GatewayRef.Name)
	}
	return nil
}

func (gatewayManager *GatewayManager) getLoadBalancerPoolSummary(ctx context.Context,
	lbPoolName string) (*swaggerClient.EdgeLoadBalancerPoolSummary, error) {
	if gatewayManager.GatewayRef == nil {
		return nil, fmt.Errorf("gateway reference should not be nil")
	}

	client := gatewayManager.Client
	// This should return exactly one result, so no need to accumulate results
	lbPoolSummaries, resp, err := client.APIClient.EdgeGatewayLoadBalancerPoolsApi.GetPoolSummariesForGateway(
		ctx, 1, 25, gatewayManager.GatewayRef.Id,
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

func (gatewayManager *GatewayManager) getLoadBalancerPool(ctx context.Context,
	lbPoolName string) (*swaggerClient.EntityReference, error) {

	lbPoolSummary, err := gatewayManager.getLoadBalancerPoolSummary(ctx, lbPoolName)
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

func (gatewayManager *GatewayManager) formLoadBalancerPool(lbPoolName string, ips []string,
	internalPort int32) (swaggerClient.EdgeLoadBalancerPool, []swaggerClient.EdgeLoadBalancerPoolMember) {
	lbPoolMembers := make([]swaggerClient.EdgeLoadBalancerPoolMember, len(ips))
	for i, ip := range ips {
		lbPoolMembers[i].IpAddress = ip
		lbPoolMembers[i].Port = internalPort
		lbPoolMembers[i].Ratio = 1
		lbPoolMembers[i].Enabled = true
	}

	lbPool := swaggerClient.EdgeLoadBalancerPool{
		Enabled:               true,
		Name:                  lbPoolName,
		DefaultPort:           internalPort,
		GracefulTimeoutPeriod: 0, // when service outage occurs, immediately mark as bad
		Members:               lbPoolMembers,
		GatewayRef:            gatewayManager.GatewayRef,
	}
	return lbPool, lbPoolMembers
}

func (gatewayManager *GatewayManager) CreateLoadBalancerPool(ctx context.Context, lbPoolName string,
	ips []string, internalPort int32) (*swaggerClient.EntityReference, error) {

	client := gatewayManager.Client
	if gatewayManager.GatewayRef == nil {
		return nil, fmt.Errorf("gateway reference should not be nil")
	}

	lbPoolRef, err := gatewayManager.getLoadBalancerPool(ctx, lbPoolName)
	if err != nil {
		return nil, fmt.Errorf("unexpected error when querying for pool [%s]: [%v]",
			lbPoolName, err)
	}
	if lbPoolRef != nil {
		klog.Infof("LoadBalancer Pool [%s] already exists", lbPoolName)
		return lbPoolRef, nil
	}

	lbPool, lbPoolMembers := gatewayManager.formLoadBalancerPool(lbPoolName, ips, internalPort)
	resp, err := client.APIClient.EdgeGatewayLoadBalancerPoolsApi.CreateLoadBalancerPool(ctx, lbPool)
	if err != nil {
		return nil, fmt.Errorf("unable to create loadbalancer pool with name [%s], members [%+v]: resp [%+v]: [%v]",
			lbPoolName, lbPoolMembers, resp, err)
	}
	if resp.StatusCode != http.StatusAccepted {
		return nil, fmt.Errorf("unable to create loadbalancer pool; expected http response [%v], obtained [%v]",
			http.StatusAccepted, resp.StatusCode)
	}

	taskURL := resp.Header.Get("Location")
	task := govcd.NewTask(&client.VCDClient.Client)
	task.Task.HREF = taskURL
	if err = task.WaitTaskCompletion(); err != nil {
		return nil, fmt.Errorf("unable to create loadbalancer pool; creation task [%s] did not complete: [%v]",
			taskURL, err)
	}

	// Get the pool to return it
	lbPoolRef, err = gatewayManager.getLoadBalancerPool(ctx, lbPoolName)
	if err != nil {
		return nil, fmt.Errorf("unexpected error when querying for pool [%s]: [%v]",
			lbPoolName, err)
	}
	if lbPoolRef == nil {
		return nil, fmt.Errorf("unable to query for loadbalancer pool [%s] that was freshly created: [%v]",
			lbPoolName, err)
	}

	klog.Infof("Created lb pool [%v] on gateway [%v]\n", lbPoolRef, gatewayManager.GatewayRef.Name)

	return lbPoolRef, nil
}

func (gatewayManager *GatewayManager) DeleteLoadBalancerPool(ctx context.Context, lbPoolName string,
	failIfAbsent bool) error {

	client := gatewayManager.Client
	if gatewayManager.GatewayRef == nil {
		return fmt.Errorf("gateway reference should not be nil")
	}

	lbPoolRef, err := gatewayManager.getLoadBalancerPool(ctx, lbPoolName)
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

	if err = gatewayManager.checkIfLBPoolIsReady(ctx, lbPoolName); err != nil {
		return err
	}

	resp, err := client.APIClient.EdgeGatewayLoadBalancerPoolApi.DeleteLoadBalancerPool(ctx, lbPoolRef.Id)
	if resp.StatusCode != http.StatusAccepted {
		return fmt.Errorf("unable to delete lb pool; expected http response [%v], obtained [%v]",
			http.StatusAccepted, resp.StatusCode)
	}

	taskURL := resp.Header.Get("Location")
	task := govcd.NewTask(&client.VCDClient.Client)
	task.Task.HREF = taskURL
	if err = task.WaitTaskCompletion(); err != nil {
		return fmt.Errorf("unable to delete lb pool; deletion task [%s] did not complete: [%v]",
			taskURL, err)
	}
	klog.Infof("Deleted loadbalancer pool [%s]\n", lbPoolName)

	return nil
}

func hasSameLBPoolMembers(array1 []swaggerClient.EdgeLoadBalancerPoolMember, array2 []string) bool {
	if array1 == nil || array2 == nil || len(array1) != len(array2) {
		return false
	}
	elementsMap := make(map[string]int)
	for _, e := range array1 {
		elementsMap[e.IpAddress] = 1
	}
	for _, e := range array2 {
		if _, ok := elementsMap[e]; !ok {
			return false
		}
	}
	return true
}

func (gatewayManager *GatewayManager) UpdateLoadBalancerPool(ctx context.Context, lbPoolName string, ips []string,
	internalPort int32) (*swaggerClient.EntityReference, error) {
	client := gatewayManager.Client
	lbPoolRef, err := gatewayManager.getLoadBalancerPool(ctx, lbPoolName)
	if err != nil {
		return nil, fmt.Errorf("unexpected error when querying for pool [%s]: [%v]", lbPoolName, err)
	}
	if lbPoolRef == nil {
		return nil, fmt.Errorf("no lb pool found with name [%s]: [%v]", lbPoolName, err)
	}

	lbPool, resp, err := client.APIClient.EdgeGatewayLoadBalancerPoolApi.GetLoadBalancerPool(ctx, lbPoolRef.Id)
	if err != nil {
		return nil, fmt.Errorf("unable to get loadbalancer pool with id [%s]: [%v]", lbPoolRef.Id, err)
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("unable to get loadbalancer pool with id [%s], expected http response [%v], obtained [%v]", lbPoolRef.Id, http.StatusOK, resp.StatusCode)
	}

	if hasSameLBPoolMembers(lbPool.Members, ips) && lbPool.Members[0].Port == internalPort {
		klog.Infof("No updates needed for the loadbalancer pool [%s]", lbPool.Name)
		return lbPoolRef, nil
	}
	if err = gatewayManager.checkIfLBPoolIsReady(ctx, lbPoolName); err != nil {
		return nil, fmt.Errorf("unable to update loadbalancer pool [%s]; loadbalancer pool is busy: [%v]", lbPoolName, err)
	}
	if err := gatewayManager.checkIfGatewayIsReady(ctx); err != nil {
		klog.Errorf("failed to update DNAT rule; gateway [%s] is busy", gatewayManager.GatewayRef.Name)
		return nil, fmt.Errorf("unable to update loadbalancer pool [%s]; gateway is busy: [%s]", lbPoolName, err)
	}
	lbPool, resp, err = client.APIClient.EdgeGatewayLoadBalancerPoolApi.GetLoadBalancerPool(ctx, lbPoolRef.Id)
	if err != nil {
		return nil, fmt.Errorf("unable to get loadbalancer pool with id [%s]: [%v]", lbPoolRef.Id, err)
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("unable to get loadbalancer pool with id [%s], expected http response [%v], obtained [%v]", lbPoolRef.Id, http.StatusOK, resp.StatusCode)
	}
	updatedLBPool, lbPoolMembers := gatewayManager.formLoadBalancerPool(lbPoolName, ips, internalPort)
	resp, err = client.APIClient.EdgeGatewayLoadBalancerPoolApi.UpdateLoadBalancerPool(ctx, updatedLBPool, lbPoolRef.Id)
	if resp != nil && resp.StatusCode != http.StatusAccepted {
		var responseMessageBytes []byte
		if gsErr, ok := err.(swaggerClient.GenericSwaggerError); ok {
			responseMessageBytes = gsErr.Body()
		}
		return nil, fmt.Errorf(
			"unable to update loadblanacer pool [%s] having members [%+v]; expected http response [%v], obtained [%v]: resp: [%#v]: [%v]",
			lbPoolName, lbPoolMembers, http.StatusAccepted, resp.StatusCode, string(responseMessageBytes), err)
	} else if err != nil {
		return nil, fmt.Errorf("unable to update loadbalancer pool having name [%s], members [%+v]: resp [%+v]: [%v]",
			lbPoolName, lbPoolMembers, resp, err)
	}

	taskURL := resp.Header.Get("Location")
	task := govcd.NewTask(&client.VCDClient.Client)
	task.Task.HREF = taskURL
	if err = task.WaitTaskCompletion(); err != nil {
		return nil, fmt.Errorf("unable to update loadbalancer pool; update task [%s] did not complete: [%v]",
			taskURL, err)
	}

	// Get the pool to return it
	lbPoolRef, err = gatewayManager.getLoadBalancerPool(ctx, lbPoolName)
	if err != nil {
		return nil, fmt.Errorf("unexpected error when querying for pool [%s]: [%v]",
			lbPoolName, err)
	}
	if lbPoolRef == nil {
		return nil, fmt.Errorf("unable to query for loadbalancer pool [%s] that was updated: [%v]",
			lbPoolName, err)
	}
	klog.Infof("Updated lb pool [%v] on gateway [%v]\n", lbPoolRef, gatewayManager.GatewayRef.Name)

	return lbPoolRef, nil
}

func (gatewayManager *GatewayManager) GetVirtualService(ctx context.Context,
	virtualServiceName string) (*swaggerClient.EdgeLoadBalancerVirtualServiceSummary, error) {

	client := gatewayManager.Client
	if gatewayManager.GatewayRef == nil {
		return nil, fmt.Errorf("gateway reference should not be nil")
	}

	// This should return exactly one result, so no need to accumulate results
	lbVSSummaries, resp, err := client.APIClient.EdgeGatewayLoadBalancerVirtualServicesApi.GetVirtualServiceSummariesForGateway(
		ctx, 1, 25, gatewayManager.GatewayRef.Id,
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

func (gatewayManager *GatewayManager) CheckIfVirtualServiceIsPending(ctx context.Context, virtualServiceName string) error {
	if gatewayManager.GatewayRef == nil {
		return fmt.Errorf("gateway reference should not be nil")
	}

	klog.V(3).Infof("Checking if virtual service [%s] is still pending", virtualServiceName)
	vsSummary, err := gatewayManager.GetVirtualService(ctx, virtualServiceName)
	if err != nil {
		return fmt.Errorf("unable to get summary for LB VS [%s]: [%v]", virtualServiceName, err)
	}
	if vsSummary == nil {
		return fmt.Errorf("unable to get summary of virtual service [%s]: [%v]", virtualServiceName, err)
	}
	if vsSummary.HealthStatus == "UP" || vsSummary.HealthStatus == "DOWN" {
		klog.V(3).Infof("Completed waiting for [%s] since healthStatus is [%s]",
			virtualServiceName, vsSummary.HealthStatus)
		return nil
	}

	klog.Errorf("Virtual service [%s] is still pending. Virtual service status: [%s]", virtualServiceName, vsSummary.HealthStatus)
	return NewVirtualServicePendingError(virtualServiceName)
}

func (gatewayManager *GatewayManager) checkIfVirtualServiceIsReady(ctx context.Context, virtualServiceName string) error {
	if gatewayManager.GatewayRef == nil {
		return fmt.Errorf("gateway reference should not be nil")
	}

	klog.V(3).Infof("Checking if virtual service [%s] is busy", virtualServiceName)
	vsSummary, err := gatewayManager.GetVirtualService(ctx, virtualServiceName)
	if err != nil {
		return fmt.Errorf("unable to get summary for LB VS [%s]: [%v]", virtualServiceName, err)
	}
	if vsSummary == nil {
		return fmt.Errorf("unable to get summary of virtual service [%s]: [%v]", virtualServiceName, err)
	}
	if *vsSummary.Status == "REALIZED" {
		klog.V(3).Infof("Completed waiting for [%s] to be configured since current status is [%s]",
			virtualServiceName, *vsSummary.Status)
		return nil
	}

	klog.Errorf("Virtual service [%s] is still being configured. Virtual service status: [%s]", virtualServiceName, *vsSummary.Status)
	return NewVirtualServiceBusyError(virtualServiceName)
}

func (gatewayManager *GatewayManager) checkIfLBPoolIsReady(ctx context.Context, lbPoolName string) error {
	if gatewayManager.GatewayRef == nil {
		return fmt.Errorf("gateway reference should not be nil")
	}

	klog.V(3).Infof("Checking if loadbalancer pool [%s] is busy", lbPoolName)
	lbPoolSummary, err := gatewayManager.getLoadBalancerPoolSummary(ctx, lbPoolName)
	if err != nil {
		return fmt.Errorf("unable to get summary for LB VS [%s]: [%v]", lbPoolName, err)
	}
	if lbPoolSummary == nil {
		return fmt.Errorf("unable to get summary of virtual service [%s]: [%v]", lbPoolName, err)
	}
	if *lbPoolSummary.Status == "REALIZED" {
		klog.V(3).Infof("Completed waiting for [%s] to be configured since load balancer pool status is [%s]",
			lbPoolName, *lbPoolSummary.Status)
		return nil
	}

	klog.Errorf("Load balancer pool [%s] is still being configured. load balancer pool status: [%s]", lbPoolName, *lbPoolSummary.Status)
	return NewLBPoolBusyError(lbPoolName)
}

func (gatewayManager *GatewayManager) checkIfGatewayIsReady(ctx context.Context) error {
	client := gatewayManager.Client
	edgeGateway, resp, err := client.APIClient.EdgeGatewayApi.GetEdgeGateway(ctx, gatewayManager.GatewayRef.Id)
	if resp != nil && resp.StatusCode != http.StatusOK {
		var responseMessageBytes []byte
		if gsErr, ok := err.(swaggerClient.GenericSwaggerError); ok {
			responseMessageBytes = gsErr.Body()
		}
		return fmt.Errorf(
			"unable to get gateway details; expected http response [%v], obtained [%v]: resp: [%#v]: [%v]",
			http.StatusOK, resp.StatusCode, string(responseMessageBytes), err)
	} else if err != nil {
		return fmt.Errorf("error while checking gateway status for [%s]: [%v]", gatewayManager.GatewayRef.Name, err)
	}
	if *edgeGateway.Status == "REALIZED" {
		klog.V(3).Infof("Completed waiting for [%s] to be configured since gateway status is [%s]",
			gatewayManager.GatewayRef.Name, *edgeGateway.Status)
		return nil
	}
	klog.Errorf("gateway [%s] is still being configured. Gateway status: [%s]", gatewayManager.GatewayRef.Name, *edgeGateway.Status)
	return NewGatewayBusyError(gatewayManager.GatewayRef.Name)
}

func (gatewayManager *GatewayManager) UpdateVirtualServicePort(ctx context.Context, virtualServiceName string, externalPort int32) (*swaggerClient.EntityReference, error) {
	client := gatewayManager.Client
	vsSummary, err := gatewayManager.GetVirtualService(ctx, virtualServiceName)
	if err != nil {
		return nil, fmt.Errorf("failed to get virtual service summary for virtual service [%s]: [%v]", virtualServiceName, err)
	}
	if vsSummary == nil {
		return nil, fmt.Errorf("virtual service [%s] doesn't exist", virtualServiceName)
	}
	if len(vsSummary.ServicePorts) == 0 {
		return nil, fmt.Errorf("virtual service [%s] has no service ports", virtualServiceName)
	}

	if vsSummary.ServicePorts[0].PortStart == externalPort {
		klog.Infof("virtual service [%s] is already configured with port [%d]", virtualServiceName, externalPort)
		return &swaggerClient.EntityReference{
			Name: vsSummary.Name,
			Id:   vsSummary.Id,
		}, nil
	}
	if err = gatewayManager.checkIfVirtualServiceIsReady(ctx, virtualServiceName); err != nil {
		return nil, err
	}
	vs, _, err := client.APIClient.EdgeGatewayLoadBalancerVirtualServiceApi.GetVirtualService(ctx, vsSummary.Id)
	if err != nil {
		return nil, fmt.Errorf("failed to get virtual service with ID [%s]", vsSummary.Id)
	}
	if externalPort != vsSummary.ServicePorts[0].PortStart {
		// update both port start and port end to be the same.
		vs.ServicePorts[0].PortStart = externalPort
		vs.ServicePorts[0].PortEnd = externalPort
	}
	resp, err := client.APIClient.EdgeGatewayLoadBalancerVirtualServiceApi.UpdateVirtualService(ctx, vs, vsSummary.Id)
	if resp != nil && resp.StatusCode != http.StatusAccepted {
		var responseMessageBytes []byte
		if gsErr, ok := err.(swaggerClient.GenericSwaggerError); ok {
			responseMessageBytes = gsErr.Body()
		}
		return nil, fmt.Errorf(
			"unable to update virtual service; expected http response [%v], obtained [%v]: resp: [%#v]: [%v]",
			http.StatusAccepted, resp.StatusCode, string(responseMessageBytes), err)
	} else if err != nil {
		return nil, fmt.Errorf("error while updating virtual service [%s]: [%v]", virtualServiceName, err)
	}
	taskURL := resp.Header.Get("Location")
	task := govcd.NewTask(&client.VCDClient.Client)
	task.Task.HREF = taskURL
	if err = task.WaitTaskCompletion(); err != nil {
		return nil, fmt.Errorf("unable to update virtual service; update task [%s] did not complete: [%v]",
			taskURL, err)
	}

	vsSummary, err = gatewayManager.GetVirtualService(ctx, virtualServiceName)
	if err != nil {
		return nil, fmt.Errorf("unable to get summary for freshly created LB VS [%s]: [%v]",
			virtualServiceName, err)
	}
	if vsSummary == nil {
		return nil, fmt.Errorf("unable to get summary of freshly created virtual service [%s]: [%v]",
			virtualServiceName, err)
	}

	klog.Errorf("successfully updated virtual service [%s] on gateway [%s]", virtualServiceName, gatewayManager.GatewayRef.Name)
	return &swaggerClient.EntityReference{
		Name: vsSummary.Name,
		Id:   vsSummary.Id,
	}, nil
}

func (gatewayManager *GatewayManager) CreateVirtualService(ctx context.Context, virtualServiceName string,
	lbPoolRef *swaggerClient.EntityReference, segRef *swaggerClient.EntityReference,
	freeIP string, vsType string, externalPort int32,
	useSSL bool, certificateAlias string) (*swaggerClient.EntityReference, error) {

	client := gatewayManager.Client
	if gatewayManager.GatewayRef == nil {
		return nil, fmt.Errorf("gateway reference should not be nil")
	}

	vsSummary, err := gatewayManager.GetVirtualService(ctx, virtualServiceName)
	if err != nil {
		return nil, fmt.Errorf("unexpected error while getting summary for LB VS [%s]: [%v]",
			virtualServiceName, err)
	}
	if vsSummary != nil {
		klog.V(3).Infof("LoadBalancer Virtual Service [%s] already exists", virtualServiceName)
		if err = gatewayManager.CheckIfVirtualServiceIsPending(ctx, virtualServiceName); err != nil {
			return nil, err
		}

		return &swaggerClient.EntityReference{
			Name: vsSummary.Name,
			Id:   vsSummary.Id,
		}, nil
	}

	if useSSL {
		klog.Infof("Creating SSL-enabled service with certificate [%s]", certificateAlias)
	}

	virtualServiceConfig := &swaggerClient.EdgeLoadBalancerVirtualService{
		Name:                  virtualServiceName,
		Enabled:               true,
		VirtualIpAddress:      freeIP,
		LoadBalancerPoolRef:   lbPoolRef,
		GatewayRef:            gatewayManager.GatewayRef,
		ServiceEngineGroupRef: segRef,
		ServicePorts: []swaggerClient.EdgeLoadBalancerServicePort{
			{
				TcpUdpProfile: &swaggerClient.EdgeLoadBalancerTcpUdpProfile{
					Type_: "TCP_PROXY",
				},
				PortStart:  externalPort,
				SslEnabled: useSSL,
			},
		},
		ApplicationProfile: &swaggerClient.EdgeLoadBalancerApplicationProfile{
			SystemDefined: true,
		},
	}
	switch vsType {
	case "TCP":
		virtualServiceConfig.ApplicationProfile.Name = "System-L4-Application"
		virtualServiceConfig.ApplicationProfile.Type_ = "L4"
		if useSSL {
			virtualServiceConfig.ApplicationProfile.Name = "System-SSL-Application"
			virtualServiceConfig.ApplicationProfile.Type_ = "L4_TLS" // this needs Enterprise License
		}
		break

	case "HTTP":
		virtualServiceConfig.ApplicationProfile.Name = "System-HTTP"
		virtualServiceConfig.ApplicationProfile.Type_ = "HTTP"
		break

	case "HTTPS":
		virtualServiceConfig.ApplicationProfile.Name = "System-Secure-HTTP"
		virtualServiceConfig.ApplicationProfile.Type_ = "HTTPS"
		break

	default:
		return nil, fmt.Errorf("unhandled virtual service type [%s]", vsType)
	}

	if useSSL {
		clusterOrg, err := client.VCDClient.GetOrgByName(client.ClusterOrgName)
		if err != nil {
			return nil, fmt.Errorf("unable to get org for org [%s]: [%v]", client.ClusterOrgName, err)
		}
		if clusterOrg == nil || clusterOrg.Org == nil {
			return nil, fmt.Errorf("obtained nil org for name [%s]", client.ClusterOrgName)
		}

		certLibItems, resp, err := client.APIClient.CertificateLibraryApi.QueryCertificateLibrary(ctx,
			1, 128,
			&swaggerClient.CertificateLibraryApiQueryCertificateLibraryOpts{
				Filter: optional.NewString(fmt.Sprintf("alias==%s", certificateAlias)),
			},
			clusterOrg.Org.ID,
		)
		if err != nil {
			return nil, fmt.Errorf("unable to get cert with alias [%s] in org [%s]: resp: [%v]: [%v]",
				certificateAlias, client.ClusterOrgName, resp, err)
		}
		if len(certLibItems.Values) != 1 {
			return nil, fmt.Errorf("expected 1 cert with alias [%s], obtained [%d]",
				certificateAlias, len(certLibItems.Values))
		}
		virtualServiceConfig.CertificateRef = &swaggerClient.EntityReference{
			Name: certLibItems.Values[0].Alias,
			Id:   certLibItems.Values[0].Id,
		}
	}

	resp, gsErr := client.APIClient.EdgeGatewayLoadBalancerVirtualServicesApi.CreateVirtualService(ctx, *virtualServiceConfig)
	if resp != nil && resp.StatusCode != http.StatusAccepted {
		return nil, fmt.Errorf(
			"unable to create virtual service; expected http response [%v], obtained [%v]: resp: [%#v]: [%v]: [%v]",
			http.StatusAccepted, resp.StatusCode, resp, gsErr, string(gsErr.Body()))
	} else if gsErr != nil {
		return nil, fmt.Errorf("error while creating virtual service [%s]: [%v]", virtualServiceName, gsErr)
	}

	taskURL := resp.Header.Get("Location")
	task := govcd.NewTask(&client.VCDClient.Client)
	task.Task.HREF = taskURL
	if err = task.WaitTaskCompletion(); err != nil {
		return nil, fmt.Errorf("unable to create virtual service; creation task [%s] did not complete: [%v]",
			taskURL, err)
	}

	vsSummary, err = gatewayManager.GetVirtualService(ctx, virtualServiceName)
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

	if err = gatewayManager.CheckIfVirtualServiceIsPending(ctx, virtualServiceName); err != nil {
		return virtualServiceRef, err
	}

	klog.Infof("Created virtual service [%v] on gateway [%v]\n", virtualServiceRef, gatewayManager.GatewayRef.Name)

	return virtualServiceRef, nil
}

func (gatewayManager *GatewayManager) DeleteVirtualService(ctx context.Context, virtualServiceName string,
	failIfAbsent bool) error {

	client := gatewayManager.Client
	if gatewayManager.GatewayRef == nil {
		return fmt.Errorf("gateway reference should not be nil")
	}

	vsSummary, err := gatewayManager.GetVirtualService(ctx, virtualServiceName)
	if err != nil {
		return fmt.Errorf("unable to get summary for LB Virtual Service [%s]: [%v]",
			virtualServiceName, err)
	}
	if vsSummary == nil {
		if failIfAbsent {
			return fmt.Errorf("virtual Service [%s] does not exist", virtualServiceName)
		}

		return nil
	}

	err = gatewayManager.checkIfVirtualServiceIsReady(ctx, virtualServiceName)
	if err != nil {
		// virtual service is busy
		return err
	}

	resp, err := client.APIClient.EdgeGatewayLoadBalancerVirtualServiceApi.DeleteVirtualService(
		ctx, vsSummary.Id)
	if resp != nil && resp.StatusCode != http.StatusAccepted {
		var responseMessageBytes []byte
		if gsErr, ok := err.(swaggerClient.GenericSwaggerError); ok {
			responseMessageBytes = gsErr.Body()
		}
		return fmt.Errorf("unable to delete virtual service [%s]; expected http response [%v], obtained [%v] with response [%v]",
			vsSummary.Name, http.StatusAccepted, resp.StatusCode, string(responseMessageBytes))
	} else if err != nil {
		return fmt.Errorf("failed to delete virtual service [%s]: [%v]", vsSummary.Name, err)
	}

	taskURL := resp.Header.Get("Location")
	task := govcd.NewTask(&client.VCDClient.Client)
	task.Task.HREF = taskURL
	if err = task.WaitTaskCompletion(); err != nil {
		return fmt.Errorf("unable to delete virtual service; deletion task [%s] did not complete: [%v]",
			taskURL, err)
	}
	klog.Infof("Deleted virtual service [%s]\n", virtualServiceName)

	return nil
}

type PortDetails struct {
	Protocol     string
	PortSuffix   string
	ExternalPort int32
	InternalPort int32
	UseSSL       bool
	CertAlias    string
}

// GetLoadBalancer :
func (gatewayManager *GatewayManager) GetLoadBalancer(ctx context.Context, virtualServiceName string, oneArm *OneArm) (string, error) {

	vsSummary, err := gatewayManager.GetVirtualService(ctx, virtualServiceName)
	if err != nil {
		return "", fmt.Errorf("unable to get summary for LB Virtual Service [%s]: [%v]",
			virtualServiceName, err)
	}
	if vsSummary == nil {
		return "", nil // this is not an error
	}

	klog.V(3).Infof("LoadBalancer Virtual Service [%s] exists", virtualServiceName)
	if err = gatewayManager.CheckIfVirtualServiceIsPending(ctx, virtualServiceName); err != nil {
		return "", err
	}

	if oneArm == nil {
		return vsSummary.VirtualIpAddress, nil
	}

	dnatRuleName := GetDNATRuleName(virtualServiceName)
	dnatRuleRef, err := gatewayManager.GetNATRuleRef(ctx, dnatRuleName)
	if err != nil {
		return "", fmt.Errorf("unable to find dnat rule [%s] for virtual service [%s]: [%v]",
			dnatRuleName, virtualServiceName, err)
	}
	if dnatRuleRef == nil {
		return "", nil // so that a retry creates the DNAT rule
	}

	return dnatRuleRef.ExternalIP, nil
}

// IsNSXTBackedGateway : return true if gateway is backed by NSX-T
func (gatewayManager *GatewayManager) IsNSXTBackedGateway() bool {
	isNSXTBackedGateway :=
		(gatewayManager.NetworkBackingType == swaggerClient.NSXT_FIXED_SEGMENT_BackingNetworkType) ||
			(gatewayManager.NetworkBackingType == swaggerClient.NSXT_FLEXIBLE_SEGMENT_BackingNetworkType)

	return isNSXTBackedGateway
}

func (gatewayManager *GatewayManager) GetLoadBalancerPool(ctx context.Context, lbPoolName string) (*swaggerClient.EntityReference, error) {
	lbPoolRef, err := gatewayManager.getLoadBalancerPool(ctx, lbPoolName)
	if err != nil {
		return nil, fmt.Errorf("unable to get reference for LB pool [%s]: [%v]", lbPoolName, err)
	}
	if lbPoolRef == nil {
		return nil, govcd.ErrorEntityNotFound
	}
	return lbPoolRef, nil
}

func (gatewayManager *GatewayManager) GetLoadBalancerPoolMemberIPs(ctx context.Context, lbPoolRef *swaggerClient.EntityReference) ([]string, error) {
	client := gatewayManager.Client
	if lbPoolRef == nil {
		return nil, govcd.ErrorEntityNotFound
	}
	lbPool, resp, err := client.APIClient.EdgeGatewayLoadBalancerPoolApi.GetLoadBalancerPool(ctx, lbPoolRef.Id)
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
