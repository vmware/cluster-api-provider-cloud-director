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
	"github.com/peterhellberg/link"
	"github.com/vmware/cloud-provider-for-cloud-director/pkg/util"
	swaggerClient "github.com/vmware/cloud-provider-for-cloud-director/pkg/vcdswaggerclient_37_2"
	"github.com/vmware/go-vcloud-director/v2/govcd"
	"github.com/vmware/go-vcloud-director/v2/types/v56"
	"k8s.io/klog"
	"math"
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
func (gm *GatewayManager) cacheGatewayDetails(ctx context.Context, ovdcName string) error {
	if gm.NetworkName == "" {
		return fmt.Errorf("network name should not be empty")
	}

	ovdcNetwork, err := gm.getOVDCNetwork(ctx, gm.NetworkName, ovdcName)
	if err != nil {
		return fmt.Errorf("unable to get OVDC network [%s]: [%v]", gm.NetworkName, err)
	}

	// Cache backing type
	if ovdcNetwork.BackingNetworkType != nil {
		gm.NetworkBackingType = *ovdcNetwork.BackingNetworkType
	}

	// Cache gateway reference
	if ovdcNetwork.Connection == nil ||
		ovdcNetwork.Connection.RouterRef == nil {
		klog.Infof("Gateway for Network Name [%s] is of type [%v]\n",
			gm.NetworkName, gm.NetworkBackingType)
		return nil
	}

	gm.GatewayRef = &swaggerClient.EntityReference{
		Name: ovdcNetwork.Connection.RouterRef.Name,
		Id:   ovdcNetwork.Connection.RouterRef.Id,
	}

	klog.Infof("Obtained Gateway [%s] for Network Name [%s] of type [%v]\n",
		gm.GatewayRef.Name, gm.NetworkName, gm.NetworkBackingType)

	return nil
}

func NewGatewayManager(ctx context.Context, client *Client, networkName string, ipamSubnet string, ovdcName string) (*GatewayManager, error) {
	if networkName == "" {
		return nil, fmt.Errorf("empty network name specified while creating GatewayManger")
	}

	gateway := GatewayManager{
		Client:      client,
		NetworkName: networkName,
		IPAMSubnet:  ipamSubnet,
	}

	err := gateway.cacheGatewayDetails(ctx, ovdcName)
	if err != nil {
		return nil, fmt.Errorf("error caching gateway related details: [%v]", err)
	}
	return &gateway, nil
}

func (gm *GatewayManager) getOVDCNetwork(ctx context.Context, networkName string, ovdcName string) (*swaggerClient.VdcNetwork, error) {
	if networkName == "" {
		return nil, fmt.Errorf("network name should not be empty")
	}

	client := gm.Client
	ovdcNetworksAPI := client.APIClient.OrgVdcNetworksApi
	pageNum := int32(1)
	ovdcNetworkID := ""
	org, err := client.VCDClient.GetOrgByName(client.ClusterOrgName)
	if err != nil {
		return nil, fmt.Errorf("error getting org by name for org [%s]: [%v]", client.ClusterOrgName, err)
	}
	if org == nil || org.Org == nil {
		return nil, fmt.Errorf("obtained nil org when getting org by name [%s]", client.ClusterOrgName)
	}
	networkFound := false
	for {
		ovdcNetworks, resp, err := ovdcNetworksAPI.GetAllVdcNetworks(ctx, org.Org.ID, pageNum, 32, nil)
		if err != nil {
			// TODO: log resp in debug mode only
			return nil, fmt.Errorf("unable to get all ovdc networks: [%+v]: [%v]", resp, err)
		}

		if len(ovdcNetworks.Values) == 0 {
			break
		}

		for _, ovdcNetwork := range ovdcNetworks.Values {
			if ovdcNetwork.Name == gm.NetworkName && (ovdcNetwork.OrgVdc == nil || ovdcNetwork.OrgVdc.Name == ovdcName) {
				if networkFound {
					return nil, fmt.Errorf("found more than one network with the name [%s] in the org [%s] - please ensure the network name is unique within an org", gm.NetworkName, client.ClusterOrgName)
				}
				ovdcNetworkID = ovdcNetwork.Id
				networkFound = true
			}
		}
		pageNum++
	}
	if ovdcNetworkID == "" {
		return nil, fmt.Errorf("unable to obtain ID for ovdc network name [%s]",
			gm.NetworkName)
	}

	ovdcNetworkAPI := client.APIClient.OrgVdcNetworkApi
	ovdcNetwork, resp, err := ovdcNetworkAPI.GetOrgVdcNetwork(ctx, ovdcNetworkID, org.Org.ID)
	if err != nil {
		return nil, fmt.Errorf("unable to get network for id [%s]: [%+v]: [%v]", ovdcNetworkID, resp, err)
	}

	return &ovdcNetwork, nil
}

// GetLoadBalancerSEG TODO: There could be a race here as we don't book a slot. Retry repeatedly to get a LB Segment.
func (gm *GatewayManager) GetLoadBalancerSEG(ctx context.Context) (*swaggerClient.EntityReference, error) {
	if gm.GatewayRef == nil {
		return nil, fmt.Errorf("gateway reference should not be nil")
	}

	client := gm.Client
	pageNum := int32(1)
	org, err := client.VCDClient.GetOrgByName(client.ClusterOrgName)
	if err != nil {
		return nil, fmt.Errorf("error getting org by name for org [%s]: [%v]", client.ClusterOrgName, err)
	}
	if org == nil || org.Org == nil {
		return nil, fmt.Errorf("obtained nil org when getting org by name [%s]", client.ClusterOrgName)
	}
	var chosenSEGAssignment *swaggerClient.LoadBalancerServiceEngineGroupAssignment = nil
	for {
		segAssignments, resp, err := client.APIClient.LoadBalancerServiceEngineGroupAssignmentsApi.GetServiceEngineGroupAssignments(
			ctx, pageNum, 25, org.Org.ID,
			&swaggerClient.LoadBalancerServiceEngineGroupAssignmentsApiGetServiceEngineGroupAssignmentsOpts{
				Filter: optional.NewString(fmt.Sprintf("gatewayRef.id==%s", gm.GatewayRef.Id)),
			},
		)
		if err != nil {
			return nil, fmt.Errorf("unable to get service engine group for gateway [%s]: resp: [%v]: [%v]",
				gm.GatewayRef.Name, resp, err)
		}
		if len(segAssignments.Values) == 0 {
			return nil, fmt.Errorf("obtained no service engine group assignment for gateway [%s]: [%v]", gm.GatewayRef.Name, err)
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

	klog.Infof("Using service engine group [%v] on gateway [%v]\n", chosenSEGAssignment.ServiceEngineGroupRef, gm.GatewayRef.Name)

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
	Name              string
	ID                string
	ExternalIP        string
	InternalIP        string
	ExternalPort      int
	InternalPort      int
	AppPortProfileRef *swaggerClient.EntityReference
}

// GetNATRuleRef returns nil if the rule is not found;
func (gm *GatewayManager) GetNATRuleRef(ctx context.Context, natRuleName string) (*NatRuleRef, error) {

	if gm.GatewayRef == nil {
		return nil, fmt.Errorf("gateway reference should not be nil")
	}
	client := gm.Client
	var natRuleRef *NatRuleRef = nil
	cursor := optional.EmptyString()
	org, err := client.VCDClient.GetOrgByName(client.ClusterOrgName)
	if err != nil {
		return nil, fmt.Errorf("error getting org by name for org [%s]: [%v]", client.ClusterOrgName, err)
	}
	if org == nil || org.Org == nil {
		return nil, fmt.Errorf("obtained nil org when getting org by name [%s]", client.ClusterOrgName)
	}
	for {
		natRules, resp, err := client.APIClient.EdgeGatewayNatRulesApi.GetNatRules(
			ctx, 128, gm.GatewayRef.Id, org.Org.ID,
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
					ID:                rule.Id,
					Name:              rule.Name,
					ExternalIP:        rule.ExternalAddresses,
					InternalIP:        rule.InternalAddresses,
					ExternalPort:      externalPort,
					InternalPort:      internalPort,
					AppPortProfileRef: rule.ApplicationPortProfile,
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

func (gm *GatewayManager) CreateAppPortProfile(appPortProfileName string, externalPort int32) (*govcd.NsxtAppPortProfile, error) {
	client := gm.Client
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

func (gm *GatewayManager) CreateDNATRule(ctx context.Context, dnatRuleName string,
	externalIP string, internalIP string, externalPort int32, internalPort int32, appPortProfile *govcd.NsxtAppPortProfile) error {

	if gm.GatewayRef == nil {
		return fmt.Errorf("gateway reference should not be nil")
	}

	client := gm.Client
	dnatRuleRef, err := gm.GetNATRuleRef(ctx, dnatRuleName)
	if err != nil {
		return fmt.Errorf("unexpected error while looking for nat rule [%s] in gateway [%s]: [%v]",
			dnatRuleName, gm.GatewayRef.Name, err)
	}
	if dnatRuleRef != nil {
		klog.Infof("DNAT Rule [%s] already exists", dnatRuleName)
		return nil
	}

	if appPortProfile == nil || appPortProfile.NsxtAppPortProfile == nil {
		return fmt.Errorf("empty app port profile")
	}

	org, err := client.VCDClient.GetOrgByName(client.ClusterOrgName)
	if err != nil {
		return fmt.Errorf("error getting org by name for org [%s]: [%v]", client.ClusterOrgName, err)
	}
	if org == nil || org.Org == nil {
		return fmt.Errorf("obtained nil org when getting org by name [%s]", client.ClusterOrgName)
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
	resp, err := client.APIClient.EdgeGatewayNatRulesApi.CreateNatRule(ctx, edgeNatRule, gm.GatewayRef.Id, org.Org.ID)
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

	dnatRuleRef, err = gm.GetNATRuleRef(ctx, dnatRuleName)
	if err != nil {
		return fmt.Errorf("unexpected error while looking for nat rule [%s] after creating it in gateway [%s]: [%v]",
			dnatRuleName, gm.GatewayRef.Name, err)
	}
	if dnatRuleRef == nil {
		return fmt.Errorf("could not find the created DNAT rule [%s] in gateway: [%v]", dnatRuleName, err)
	}

	klog.Infof("Created DNAT rule [%s]: [%s:%d] => [%s:%d] on gateway [%s]\n", dnatRuleName,
		externalIP, externalPort, internalIP, internalPort, gm.GatewayRef.Name)

	return nil
}

func (gm *GatewayManager) UpdateAppPortProfile(appPortProfileName string, externalPort int32) (*govcd.NsxtAppPortProfile, error) {
	client := gm.Client
	org, err := client.VCDClient.GetOrgByName(client.ClusterOrgName)
	if err != nil {
		return nil, fmt.Errorf("unable to find org [%s] by name: [%v]", client.ClusterOrgName, err)
	}
	appPortProfile, err := org.GetNsxtAppPortProfileByName(appPortProfileName, types.ApplicationPortProfileScopeTenant)
	if err != nil {
		klog.Errorf("NSX-T app port profile with the name [%s] is not found: [%v]", appPortProfileName, err)
		return nil, govcd.ErrorEntityNotFound
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

func (gm *GatewayManager) UpdateDNATRule(ctx context.Context, dnatRuleName string, externalIP string, internalIP string, externalPort int32) (*NatRuleRef, error) {
	client := gm.Client
	if err := gm.checkIfGatewayIsReady(ctx); err != nil {
		klog.Errorf("failed to update DNAT rule; gateway [%s] is busy", gm.GatewayRef.Name)
		return nil, err
	}
	dnatRuleRef, err := gm.GetNATRuleRef(ctx, dnatRuleName)
	if err != nil {
		return nil, fmt.Errorf("unexpected error while looking for nat rule [%s] in gateway [%s]: [%v]",
			dnatRuleName, gm.GatewayRef.Name, err)
	}
	org, err := client.VCDClient.GetOrgByName(client.ClusterOrgName)
	if err != nil {
		return nil, fmt.Errorf("error getting org by name for org [%s]: [%v]", client.ClusterOrgName, err)
	}
	if org == nil || org.Org == nil {
		return nil, fmt.Errorf("obtained nil org when getting org by name [%s]", client.ClusterOrgName)
	}
	if dnatRuleRef == nil {
		return nil, fmt.Errorf("failed to get DNAT rule name [%s]", dnatRuleName)
	}
	dnatRule, resp, err := client.APIClient.EdgeGatewayNatRuleApi.GetNatRule(ctx, gm.GatewayRef.Id, dnatRuleRef.ID, org.Org.ID)
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
	resp, err = client.APIClient.EdgeGatewayNatRuleApi.UpdateNatRule(ctx, dnatRule, gm.GatewayRef.Id, dnatRuleRef.ID, org.Org.ID)
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

	dnatRuleRef, err = gm.GetNATRuleRef(ctx, dnatRuleName)
	if err != nil {
		return nil, fmt.Errorf("unexpected error while looking for nat rule [%s] in gateway [%s]: [%v]",
			dnatRuleName, gm.GatewayRef.Name, err)
	}
	if dnatRuleRef == nil {
		return nil, fmt.Errorf("failed to get DNAT rule name [%s]", dnatRuleName)
	}

	klog.Infof("successfully updated DNAT rule [%s] on gateway [%s]", dnatRuleRef.Name, gm.GatewayRef.Name)
	return dnatRuleRef, nil
}

// DeleteAppPortProfile deletes app port profile if present. No error is returned if the app port profile with
// the provided name is not present in VCD
func (gm *GatewayManager) DeleteAppPortProfile(appPortProfileName string, failIfAbsent bool) error {
	client := gm.Client
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

// DeleteDNATRule Note that this also deletes App Port Profile Config. So we always need to call this
// even if we don't find a DNAT rule, to ensure that everything is cleaned up.
func (gm *GatewayManager) DeleteDNATRule(ctx context.Context, dnatRuleName string,
	failIfAbsent bool) error {

	client := gm.Client
	if err := gm.checkIfGatewayIsReady(ctx); err != nil {
		klog.Errorf("failed to update DNAT rule; gateway [%s] is busy", gm.GatewayRef.Name)
		return err
	}

	if gm.GatewayRef == nil {
		return fmt.Errorf("gateway reference should not be nil")
	}

	org, err := client.VCDClient.GetOrgByName(client.ClusterOrgName)
	if err != nil {
		return fmt.Errorf("error getting org by name for org [%s]: [%v]", client.ClusterOrgName, err)
	}
	if org == nil || org.Org == nil {
		return fmt.Errorf("obtained nil org when getting org by name [%s]", client.ClusterOrgName)
	}
	dnatRuleRef, err := gm.GetNATRuleRef(ctx, dnatRuleName)
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
			gm.GatewayRef.Id, dnatRuleRef.ID, org.Org.ID)
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
		klog.Infof("Deleted DNAT rule [%s] on gateway [%s]\n", dnatRuleName, gm.GatewayRef.Name)
	}
	return nil
}

func (gm *GatewayManager) getLoadBalancerPoolSummary(ctx context.Context,
	lbPoolName string) (*swaggerClient.EdgeLoadBalancerPoolSummary, error) {
	if gm.GatewayRef == nil {
		return nil, fmt.Errorf("gateway reference should not be nil")
	}

	client := gm.Client
	org, err := client.VCDClient.GetOrgByName(client.ClusterOrgName)
	if err != nil {
		return nil, fmt.Errorf("error getting org by name for org [%s]: [%v]", client.ClusterOrgName, err)
	}
	if org == nil || org.Org == nil {
		return nil, fmt.Errorf("obtained nil org when getting org by name [%s]", client.ClusterOrgName)
	}
	// This should return exactly one result, so no need to accumulate results
	lbPoolSummaries, resp, err := client.APIClient.EdgeGatewayLoadBalancerPoolsApi.GetPoolSummariesForGateway(
		ctx, 1, 25, gm.GatewayRef.Id, org.Org.ID,
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

func (gm *GatewayManager) getLoadBalancerPool(ctx context.Context,
	lbPoolName string) (*swaggerClient.EntityReference, error) {

	lbPoolSummary, err := gm.getLoadBalancerPoolSummary(ctx, lbPoolName)
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

func (gm *GatewayManager) formLoadBalancerPool(lbPoolName string, ips []string, internalPort int32,
	healthMonitor *swaggerClient.EdgeLoadBalancerHealthMonitor) (swaggerClient.EdgeLoadBalancerPool,
	[]swaggerClient.EdgeLoadBalancerPoolMember) {
	lbPoolMembers := make([]swaggerClient.EdgeLoadBalancerPoolMember, len(ips))
	for i, ip := range ips {
		lbPoolMembers[i].IpAddress = ip
		lbPoolMembers[i].Port = internalPort
		lbPoolMembers[i].Ratio = 1
		lbPoolMembers[i].Enabled = true
	}

	// TODO: add health monitors and persistence profile
	lbPool := swaggerClient.EdgeLoadBalancerPool{
		Enabled:               true,
		Name:                  lbPoolName,
		DefaultPort:           internalPort,
		Members:               lbPoolMembers,
		GatewayRef:            gm.GatewayRef,
		GracefulTimeoutPeriod: int32(0), // when service outage occurs, immediately mark as bad
		Algorithm:             "ROUND_ROBIN",
	}
	if healthMonitor != nil && healthMonitor.Type_ == "TCP" {
		lbPool.HealthMonitors = []swaggerClient.EdgeLoadBalancerHealthMonitor{*healthMonitor}
	}

	return lbPool, lbPoolMembers
}

func (gm *GatewayManager) CreateLoadBalancerPool(ctx context.Context, lbPoolName string, lbPoolIPList []string,
	internalPort int32, protocol string) (*swaggerClient.EntityReference, error) {

	client := gm.Client
	if gm.GatewayRef == nil {
		return nil, fmt.Errorf("gateway reference should not be nil")
	}

	org, err := client.VCDClient.GetOrgByName(client.ClusterOrgName)
	if err != nil {
		return nil, fmt.Errorf("error getting org by name for org [%s]: [%v]", client.ClusterOrgName, err)
	}
	if org == nil || org.Org == nil {
		return nil, fmt.Errorf("obtained nil org when getting org by name [%s]", client.ClusterOrgName)
	}

	lbPoolRef, err := gm.getLoadBalancerPool(ctx, lbPoolName)
	if err != nil {
		return nil, fmt.Errorf("unexpected error when querying for pool [%s]: [%v]",
			lbPoolName, err)
	}
	if lbPoolRef != nil {
		klog.Infof("LoadBalancer Pool [%s] already exists", lbPoolName)
		return lbPoolRef, nil
	}

	var healthMonitor *swaggerClient.EdgeLoadBalancerHealthMonitor = nil
	if protocol != "" {
		healthMonitor = &swaggerClient.EdgeLoadBalancerHealthMonitor{Type_: protocol}
	}
	lbPoolUniqueIPList := util.NewSet(lbPoolIPList).GetElements()
	lbPool, lbPoolMembers := gm.formLoadBalancerPool(lbPoolName, lbPoolUniqueIPList, internalPort, healthMonitor)
	resp, err := client.APIClient.EdgeGatewayLoadBalancerPoolsApi.CreateLoadBalancerPool(ctx, lbPool, org.Org.ID)

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
	lbPoolRef, err = gm.getLoadBalancerPool(ctx, lbPoolName)
	if err != nil {
		return nil, fmt.Errorf("unexpected error when querying for pool [%s]: [%v]",
			lbPoolName, err)
	}
	if lbPoolRef == nil {
		return nil, fmt.Errorf("unable to query for loadbalancer pool [%s] that was freshly created: [%v]",
			lbPoolName, err)
	}

	klog.Infof("Created lb pool [%v] on gateway [%v]\n", lbPoolRef, gm.GatewayRef.Name)

	return lbPoolRef, nil
}

func (gm *GatewayManager) DeleteLoadBalancerPool(ctx context.Context, lbPoolName string,
	failIfAbsent bool) error {

	client := gm.Client
	if gm.GatewayRef == nil {
		return fmt.Errorf("gateway reference should not be nil")
	}

	lbPoolRef, err := gm.getLoadBalancerPool(ctx, lbPoolName)
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
	org, err := client.VCDClient.GetOrgByName(client.ClusterOrgName)
	if err != nil {
		return fmt.Errorf("error getting org by name for org [%s]: [%v]", client.ClusterOrgName, err)
	}
	if org == nil || org.Org == nil {
		return fmt.Errorf("obtained nil org when getting org by name [%s]", client.ClusterOrgName)
	}

	if err = gm.checkIfLBPoolIsReady(ctx, lbPoolName); err != nil {
		return err
	}

	resp, err := client.APIClient.EdgeGatewayLoadBalancerPoolApi.DeleteLoadBalancerPool(ctx, lbPoolRef.Id, org.Org.ID)
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

func (gm *GatewayManager) UpdateLoadBalancerPool(ctx context.Context, lbPoolName string, lbPoolIPList []string,
	internalPort int32, protocol string) (*swaggerClient.EntityReference, error) {
	client := gm.Client
	lbPoolRef, err := gm.getLoadBalancerPool(ctx, lbPoolName)
	if err != nil {
		return nil, fmt.Errorf("unexpected error when querying for pool [%s]: [%v]", lbPoolName, err)
	}
	if lbPoolRef == nil {
		return nil, fmt.Errorf("no lb pool found with name [%s]: [%v]", lbPoolName, err)
	}

	org, err := client.VCDClient.GetOrgByName(client.ClusterOrgName)
	if err != nil {
		return nil, fmt.Errorf("error getting org by name for org [%s]: [%v]", client.ClusterOrgName, err)
	}
	if org == nil || org.Org == nil {
		return nil, fmt.Errorf("obtained nil org when getting org by name [%s]", client.ClusterOrgName)
	}

	lbPool, resp, err := client.APIClient.EdgeGatewayLoadBalancerPoolApi.GetLoadBalancerPool(ctx, lbPoolRef.Id, org.Org.ID)
	if err != nil {
		return nil, fmt.Errorf("unable to get loadbalancer pool with id [%s]: [%v]", lbPoolRef.Id, err)
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("unable to get loadbalancer pool with id [%s], expected http response [%v], obtained [%v]", lbPoolRef.Id, http.StatusOK, resp.StatusCode)
	}

	lbPoolUniqueIPList := util.NewSet(lbPoolIPList).GetElements()
	if hasSameLBPoolMembers(lbPool.Members, lbPoolUniqueIPList) && lbPool.Members[0].Port == internalPort {
		klog.Infof("No updates needed for the loadbalancer pool [%s]", lbPool.Name)
		return lbPoolRef, nil
	}
	if err = gm.checkIfLBPoolIsReady(ctx, lbPoolName); err != nil {
		return nil, fmt.Errorf("unable to update loadbalancer pool [%s]; loadbalancer pool is busy: [%v]", lbPoolName, err)
	}
	if err := gm.checkIfGatewayIsReady(ctx); err != nil {
		klog.Errorf("failed to update DNAT rule; gateway [%s] is busy", gm.GatewayRef.Name)
		return nil, fmt.Errorf("unable to update loadbalancer pool [%s]; gateway is busy: [%s]", lbPoolName, err)
	}
	lbPool, resp, err = client.APIClient.EdgeGatewayLoadBalancerPoolApi.GetLoadBalancerPool(ctx, lbPoolRef.Id, org.Org.ID)
	if err != nil {
		return nil, fmt.Errorf("unable to get loadbalancer pool with id [%s]: [%v]", lbPoolRef.Id, err)
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("unable to get loadbalancer pool with id [%s], expected http response [%v], obtained [%v]", lbPoolRef.Id, http.StatusOK, resp.StatusCode)
	}

	var healthMonitor *swaggerClient.EdgeLoadBalancerHealthMonitor = nil
	if protocol != "" {
		healthMonitor = &swaggerClient.EdgeLoadBalancerHealthMonitor{Type_: protocol}
	}
	updatedLBPool, lbPoolMembers := gm.formLoadBalancerPool(lbPoolName, lbPoolUniqueIPList, internalPort, healthMonitor)
	resp, err = client.APIClient.EdgeGatewayLoadBalancerPoolApi.UpdateLoadBalancerPool(ctx, updatedLBPool, lbPoolRef.Id, org.Org.ID)
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
	lbPoolRef, err = gm.getLoadBalancerPool(ctx, lbPoolName)
	if err != nil {
		return nil, fmt.Errorf("unexpected error when querying for pool [%s]: [%v]",
			lbPoolName, err)
	}
	if lbPoolRef == nil {
		return nil, fmt.Errorf("unable to query for loadbalancer pool [%s] that was updated: [%v]",
			lbPoolName, err)
	}
	klog.Infof("Updated lb pool [%v] on gateway [%v]\n", lbPoolRef, gm.GatewayRef.Name)

	return lbPoolRef, nil
}

func (gm *GatewayManager) GetVirtualService(ctx context.Context,
	virtualServiceName string) (*swaggerClient.EdgeLoadBalancerVirtualServiceSummary, error) {

	client := gm.Client
	if gm.GatewayRef == nil {
		return nil, fmt.Errorf("gateway reference should not be nil")
	}

	org, err := client.VCDClient.GetOrgByName(client.ClusterOrgName)
	if err != nil {
		return nil, fmt.Errorf("error getting org by name for org [%s]: [%v]", client.ClusterOrgName, err)
	}
	if org == nil || org.Org == nil {
		return nil, fmt.Errorf("obtained nil org when getting org by name [%s]", client.ClusterOrgName)
	}
	// This should return exactly one result, so no need to accumulate results
	lbVSSummaries, resp, err := client.APIClient.EdgeGatewayLoadBalancerVirtualServicesApi.GetVirtualServiceSummariesForGateway(
		ctx, 1, 25, gm.GatewayRef.Id, org.Org.ID,
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

func (gm *GatewayManager) CheckIfVirtualServiceIsPending(ctx context.Context, virtualServiceName string) error {
	if gm.GatewayRef == nil {
		return fmt.Errorf("gateway reference should not be nil")
	}

	klog.V(3).Infof("Checking if virtual service [%s] is still pending", virtualServiceName)
	vsSummary, err := gm.GetVirtualService(ctx, virtualServiceName)
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

func (gm *GatewayManager) checkIfVirtualServiceIsReady(ctx context.Context, virtualServiceName string) error {
	if gm.GatewayRef == nil {
		return fmt.Errorf("gateway reference should not be nil")
	}

	klog.V(3).Infof("Checking if virtual service [%s] is busy", virtualServiceName)
	vsSummary, err := gm.GetVirtualService(ctx, virtualServiceName)
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

func (gm *GatewayManager) checkIfLBPoolIsReady(ctx context.Context, lbPoolName string) error {
	if gm.GatewayRef == nil {
		return fmt.Errorf("gateway reference should not be nil")
	}

	klog.V(3).Infof("Checking if loadbalancer pool [%s] is busy", lbPoolName)
	lbPoolSummary, err := gm.getLoadBalancerPoolSummary(ctx, lbPoolName)
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

func (gm *GatewayManager) checkIfGatewayIsReady(ctx context.Context) error {
	client := gm.Client
	org, err := client.VCDClient.GetOrgByName(client.ClusterOrgName)
	if err != nil {
		return fmt.Errorf("error getting org by name for org [%s]: [%v]", client.ClusterOrgName, err)
	}
	if org == nil || org.Org == nil {
		return fmt.Errorf("obtained nil org when getting org by name [%s]", client.ClusterOrgName)
	}
	edgeGateway, resp, err := client.APIClient.EdgeGatewayApi.GetEdgeGateway(ctx, gm.GatewayRef.Id, org.Org.ID)
	if resp != nil && resp.StatusCode != http.StatusOK {
		var responseMessageBytes []byte
		if gsErr, ok := err.(swaggerClient.GenericSwaggerError); ok {
			responseMessageBytes = gsErr.Body()
		}
		return fmt.Errorf(
			"unable to get gateway details; expected http response [%v], obtained [%v]: resp: [%#v]: [%v]",
			http.StatusOK, resp.StatusCode, string(responseMessageBytes), err)
	} else if err != nil {
		return fmt.Errorf("error while checking gateway status for [%s]: [%v]", gm.GatewayRef.Name, err)
	}
	if *edgeGateway.Status == "REALIZED" {
		klog.V(3).Infof("Completed waiting for [%s] to be configured since gateway status is [%s]",
			gm.GatewayRef.Name, *edgeGateway.Status)
		return nil
	}
	klog.Errorf("gateway [%s] is still being configured. Gateway status: [%s]", gm.GatewayRef.Name, *edgeGateway.Status)
	return NewGatewayBusyError(gm.GatewayRef.Name)
}

func (gm *GatewayManager) UpdateVirtualService(ctx context.Context, virtualServiceName string,
	virtualServiceIP string, externalPort int32, oneArmEnabled bool) (*swaggerClient.EntityReference, error) {
	client := gm.Client
	vsSummary, err := gm.GetVirtualService(ctx, virtualServiceName)
	if err != nil {
		return nil, fmt.Errorf("failed to get virtual service summary for virtual service [%s]: [%v]", virtualServiceName, err)
	}
	if vsSummary == nil {
		return nil, fmt.Errorf("virtual service [%s] doesn't exist", virtualServiceName)
	}
	if len(vsSummary.ServicePorts) == 0 {
		return nil, fmt.Errorf("virtual service [%s] has no service ports", virtualServiceName)
	}
	org, err := client.VCDClient.GetOrgByName(client.ClusterOrgName)
	if err != nil {
		return nil, fmt.Errorf("error getting org by name for org [%s]: [%v]", client.ClusterOrgName, err)
	}
	if org == nil || org.Org == nil {
		return nil, fmt.Errorf("obtained nil org when getting org by name [%s]", client.ClusterOrgName)
	}

	if vsSummary.ServicePorts[0].PortStart == externalPort && vsSummary.VirtualIpAddress == virtualServiceIP {
		klog.Infof("virtual service [%s] is already configured with port [%d] and virtual IP [%s]", virtualServiceName, externalPort, virtualServiceIP)
		return &swaggerClient.EntityReference{
			Name: vsSummary.Name,
			Id:   vsSummary.Id,
		}, nil
	}
	if err = gm.checkIfVirtualServiceIsReady(ctx, virtualServiceName); err != nil {
		return nil, err
	}
	vs, _, err := client.APIClient.EdgeGatewayLoadBalancerVirtualServiceApi.GetVirtualService(ctx, vsSummary.Id, org.Org.ID)
	if err != nil {
		return nil, fmt.Errorf("failed to get virtual service with ID [%s]", vsSummary.Id)
	}
	if externalPort != vsSummary.ServicePorts[0].PortStart {
		// update both port start and port end to be the same.
		vs.ServicePorts[0].PortStart = externalPort
		vs.ServicePorts[0].PortEnd = externalPort
	}
	if virtualServiceIP != "" && !oneArmEnabled && vs.VirtualIpAddress != virtualServiceIP {
		// update the virtual IP address of the virtual service when one arm is nil
		vs.VirtualIpAddress = virtualServiceIP
	}
	resp, err := client.APIClient.EdgeGatewayLoadBalancerVirtualServiceApi.UpdateVirtualService(ctx, vs, vsSummary.Id, org.Org.ID)
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

	vsSummary, err = gm.GetVirtualService(ctx, virtualServiceName)
	if err != nil {
		return nil, fmt.Errorf("unable to get summary for freshly created LB VS [%s]: [%v]",
			virtualServiceName, err)
	}
	if vsSummary == nil {
		return nil, fmt.Errorf("unable to get summary of freshly created virtual service [%s]: [%v]",
			virtualServiceName, err)
	}

	klog.Errorf("successfully updated virtual service [%s] on gateway [%s]", virtualServiceName, gm.GatewayRef.Name)
	return &swaggerClient.EntityReference{
		Name: vsSummary.Name,
		Id:   vsSummary.Id,
	}, nil
}

func (gm *GatewayManager) CreateVirtualService(ctx context.Context, virtualServiceName string,
	lbPoolRef *swaggerClient.EntityReference, segRef *swaggerClient.EntityReference,
	freeIP string, vsType string, externalPort int32,
	useSSL bool, certificateAlias string) (*swaggerClient.EntityReference, error) {

	client := gm.Client
	if gm.GatewayRef == nil {
		return nil, fmt.Errorf("gateway reference should not be nil")
	}

	vsSummary, err := gm.GetVirtualService(ctx, virtualServiceName)
	if err != nil {
		return nil, fmt.Errorf("unexpected error while getting summary for LB VS [%s]: [%v]",
			virtualServiceName, err)
	}
	if vsSummary != nil {
		klog.V(3).Infof("LoadBalancer Virtual Service [%s] already exists", virtualServiceName)
		if err = gm.CheckIfVirtualServiceIsPending(ctx, virtualServiceName); err != nil {
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
		GatewayRef:            gm.GatewayRef,
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

	clusterOrg, err := client.VCDClient.GetOrgByName(client.ClusterOrgName)
	if err != nil {
		return nil, fmt.Errorf("unable to get org for org [%s]: [%v]", client.ClusterOrgName, err)
	}
	if clusterOrg == nil || clusterOrg.Org == nil {
		return nil, fmt.Errorf("obtained nil org for name [%s]", client.ClusterOrgName)
	}
	if useSSL {
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

	resp, gsErr := client.APIClient.EdgeGatewayLoadBalancerVirtualServicesApi.CreateVirtualService(ctx, *virtualServiceConfig, clusterOrg.Org.ID)
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

	vsSummary, err = gm.GetVirtualService(ctx, virtualServiceName)
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

	if err = gm.CheckIfVirtualServiceIsPending(ctx, virtualServiceName); err != nil {
		return virtualServiceRef, err
	}

	klog.Infof("Created virtual service [%v] on gateway [%v]\n", virtualServiceRef, gm.GatewayRef.Name)

	return virtualServiceRef, nil
}

func (gm *GatewayManager) DeleteVirtualService(ctx context.Context, virtualServiceName string,
	failIfAbsent bool) error {

	client := gm.Client
	if gm.GatewayRef == nil {
		return fmt.Errorf("gateway reference should not be nil")
	}

	vsSummary, err := gm.GetVirtualService(ctx, virtualServiceName)
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
	clusterOrg, err := client.VCDClient.GetOrgByName(client.ClusterOrgName)
	if err != nil {
		return fmt.Errorf("unable to get org for org [%s]: [%v]", client.ClusterOrgName, err)
	}
	if clusterOrg == nil || clusterOrg.Org == nil {
		return fmt.Errorf("obtained nil org for name [%s]", client.ClusterOrgName)
	}

	err = gm.checkIfVirtualServiceIsReady(ctx, virtualServiceName)
	if err != nil {
		// virtual service is busy
		return err
	}

	resp, err := client.APIClient.EdgeGatewayLoadBalancerVirtualServiceApi.DeleteVirtualService(
		ctx, vsSummary.Id, clusterOrg.Org.ID)
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
func (gm *GatewayManager) GetLoadBalancer(ctx context.Context, virtualServiceName string, lbPoolName string, oneArm *OneArm) (string, *util.AllocatedResourcesMap, error) {

	allocatedResources := util.AllocatedResourcesMap{}
	vsSummary, err := gm.GetVirtualService(ctx, virtualServiceName)
	if err != nil {
		return "", nil, fmt.Errorf("unable to get summary for LB Virtual Service [%s]: [%v]",
			virtualServiceName, err)
	}
	if vsSummary == nil {
		return "", &allocatedResources, nil // this is not an error
	}
	allocatedResources.Insert(VcdResourceVirtualService, &swaggerClient.EntityReference{
		Name: vsSummary.Name,
		Id:   vsSummary.Id,
	})

	lbPoolRef, err := gm.GetLoadBalancerPool(ctx, lbPoolName)
	if err != nil {
		return "", nil, fmt.Errorf("unable to get load balancer pool information for LB Pool [%s]: [%v]", lbPoolName, err)
	}
	// no need to check if lbPoolRef is nil. Since the function GetLoadBalancerPool returns an error if lbPoolRef returned is nil
	allocatedResources.Insert(VcdResourceLoadBalancerPool, lbPoolRef)

	klog.V(3).Infof("LoadBalancer Virtual Service [%s] exists", virtualServiceName)
	if err = gm.CheckIfVirtualServiceIsPending(ctx, virtualServiceName); err != nil {
		return "", &allocatedResources, err
	}

	if oneArm == nil {
		return vsSummary.VirtualIpAddress, &allocatedResources, nil
	}

	dnatRuleName := GetDNATRuleName(virtualServiceName)
	dnatRuleRef, err := gm.GetNATRuleRef(ctx, dnatRuleName)
	if err != nil {
		return "", &allocatedResources, fmt.Errorf("unable to find dnat rule [%s] for virtual service [%s]: [%v]",
			dnatRuleName, virtualServiceName, err)
	}
	if dnatRuleRef == nil {
		return "", &allocatedResources, nil // so that a retry creates the DNAT rule
	}
	allocatedResources.Insert(VcdResourceDNATRule, &swaggerClient.EntityReference{
		Name: dnatRuleRef.Name,
		Id:   dnatRuleRef.ID,
	})

	if dnatRuleRef.AppPortProfileRef != nil {
		allocatedResources.Insert(VcdResourceAppPortProfile, &swaggerClient.EntityReference{
			Name: dnatRuleRef.AppPortProfileRef.Name,
			Id:   dnatRuleRef.AppPortProfileRef.Id,
		})
	}

	return dnatRuleRef.ExternalIP, &allocatedResources, nil
}

// IsNSXTBackedGateway : return true if gateway is backed by NSX-T
func (gm *GatewayManager) IsNSXTBackedGateway() bool {
	isNSXTBackedGateway :=
		(gm.NetworkBackingType == swaggerClient.NSXT_FIXED_SEGMENT_BackingNetworkType) ||
			(gm.NetworkBackingType == swaggerClient.NSXT_FLEXIBLE_SEGMENT_BackingNetworkType)

	return isNSXTBackedGateway
}

// IsUsingIpSpaces Returns true if the gateway is using Ip Spaces, returns false if the gateway is using Ip blocks
// in case the code is unable to determine the required info, it will return false with an error.
func (gm *GatewayManager) IsUsingIpSpaces() (bool, error) {
	edgeGatewayName := gm.GatewayRef.Name
	edgeGatewayID := gm.GatewayRef.Id
	clusterOrg, err := gm.Client.VCDClient.GetOrgByName(gm.Client.ClusterOrgName)
	if err != nil {
		return false, fmt.Errorf("error retrieving org [%s]: [%v]", gm.Client.ClusterOrgName, err)
	}
	if clusterOrg == nil || clusterOrg.Org == nil {
		return false, fmt.Errorf("unable to determine if gateway [%s] is using Ip Spaces or not; obtained nil org", edgeGatewayName)
	}
	edgeGateway, err := clusterOrg.GetNsxtEdgeGatewayById(edgeGatewayID)
	if err == nil {
		return false, fmt.Errorf("unable to determine if gateway [%s] is using Ip Spaces or not. error [%v]", edgeGatewayName, err)
	}

	edgeGatewayUplinks := edgeGateway.EdgeGateway.EdgeGatewayUplinks
	if edgeGatewayUplinks != nil {
		if len(edgeGatewayUplinks) != 1 {
			return false, fmt.Errorf("found invalid number of uplinks for gateway [%s], expecting 1", edgeGatewayName)
		}
	} else {
		return false, fmt.Errorf("no uplinks were found for gateway [%s], expecting 1", edgeGatewayName)
	}

	result := edgeGatewayUplinks[0].UsingIpSpace
	if result == nil {
		return false, fmt.Errorf("unable to determine if gateway [%s] is using IP spaces or not", edgeGatewayName)
	}
	return *result, nil
}

func (gm *GatewayManager) GetLoadBalancerPool(ctx context.Context, lbPoolName string) (*swaggerClient.EntityReference, error) {
	lbPoolRef, err := gm.getLoadBalancerPool(ctx, lbPoolName)
	if err != nil {
		return nil, fmt.Errorf("unable to get reference for LB pool [%s]: [%v]", lbPoolName, err)
	}
	if lbPoolRef == nil {
		return nil, govcd.ErrorEntityNotFound
	}
	return lbPoolRef, nil
}

func (gm *GatewayManager) GetLoadBalancerPoolMemberIPs(ctx context.Context, lbPoolRef *swaggerClient.EntityReference) ([]string, error) {
	client := gm.Client
	if lbPoolRef == nil {
		return nil, govcd.ErrorEntityNotFound
	}
	clusterOrg, err := client.VCDClient.GetOrgByName(client.ClusterOrgName)
	if err != nil {
		return nil, fmt.Errorf("unable to get org for org [%s]: [%v]", client.ClusterOrgName, err)
	}
	if clusterOrg == nil || clusterOrg.Org == nil {
		return nil, fmt.Errorf("obtained nil org for name [%s]", client.ClusterOrgName)
	}
	lbPool, resp, err := client.APIClient.EdgeGatewayLoadBalancerPoolApi.GetLoadBalancerPool(ctx, lbPoolRef.Id, clusterOrg.Org.ID)
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

func (gm *GatewayManager) CreateLoadBalancer(
	ctx context.Context, virtualServiceNamePrefix string, lbPoolNamePrefix string, lbIpClaimMarker string,
	ips []string, portDetailsList []PortDetails, oneArm *OneArm, enableVirtualServiceSharedIP bool,
	portNameToIP map[string]string, providedIP string, resourcesAllocated *util.AllocatedResourcesMap) (string, error) {
	if len(portDetailsList) == 0 {
		// nothing to do here
		klog.Infof("There is no port specified. Hence nothing to do.")
		return "", fmt.Errorf("nothing to do since http and https ports are not specified")
	}

	if gm.GatewayRef == nil {
		return "", fmt.Errorf("gateway reference should not be nil")
	}

	klog.Infof("Using provided IP [%s]\n", providedIP)

	client := gm.Client
	client.RWLock.Lock()
	defer client.RWLock.Unlock()

	// get shared ip when vsSharedIP is true and portNameToIP is not nil
	sharedVirtualIP := ""
	portNamesToCreate := make(map[string]bool) // golang does not have set data structure
	if enableVirtualServiceSharedIP && portNameToIP != nil {
		for portName, ip := range portNameToIP {
			if ip != "" {
				sharedVirtualIP = ip
			} else {
				portNamesToCreate[portName] = true
			}
		}
	}

	// Separately loop through all DNAT rules to see if any exist, so that we can reuse the external IP in case a
	// partial creation of load-balancer is continued and an externalIP was claimed earlier by a dnat rule
	externalIP := providedIP
	sharedInternalIP := ""
	var err error
	if oneArm != nil {
		for _, portDetails := range portDetailsList {
			if portDetails.InternalPort == 0 {
				continue
			}

			virtualServiceName := fmt.Sprintf("%s-%s", virtualServiceNamePrefix, portDetails.PortSuffix)
			dnatRuleName := GetDNATRuleName(virtualServiceName)
			dnatRuleRef, err := gm.GetNATRuleRef(ctx, dnatRuleName)
			if err != nil {
				return "", fmt.Errorf("unable to retrieve created dnat rule [%s]: [%v]", dnatRuleName, err)
			}
			if dnatRuleRef == nil {
				continue // ths implies that the rule does not exist
			}

			if externalIP != "" && externalIP != dnatRuleRef.ExternalIP {
				return "", fmt.Errorf("as per dnat there are two external IP rules for the same service: [%s], [%s]",
					externalIP, dnatRuleRef.ExternalIP)
			}

			externalIP = dnatRuleRef.ExternalIP
			if enableVirtualServiceSharedIP {
				sharedInternalIP = dnatRuleRef.InternalIP
			}
		}
	}

	// 3 variables: enableVirtualServiceSharedIP, oneArm, sharedIP
	// if enableVirtualServiceSharedIP is true and oneArm is nil, no internal ip is used
	// if enableVirtualServiceSharedIP is true and oneArm is not nil, an internal ip will be used and shared
	// if enableVirtualServiceSharedIP is false and oneArm is nil: this is an error case which is handled earlier in ValidateCloudConfig.
	// if enableVirtualServiceSharedIP is false and oneArm is not nil, a pair of internal IPs will be used and not shared
	if enableVirtualServiceSharedIP && oneArm == nil { // no internal ip used so no dnat rule needed
		if sharedVirtualIP != "" { // shared virtual ip is an external ip
			externalIP = sharedVirtualIP
		}
	} else if enableVirtualServiceSharedIP && oneArm != nil { // internal ip used, dnat rule is needed
		if sharedInternalIP == "" { // no dnat rule has been created yet
			sharedInternalIP, err = gm.GetUnusedInternalIPAddress(ctx, oneArm)
			if err != nil {
				return "", fmt.Errorf("unable to get internal IP address for one-arm mode: [%v]", err)
			}
		}
	}

	if externalIP == "" {
		isGatewayUsingIpSpaces, err := gm.IsUsingIpSpaces()
		if err != nil {
			return "", fmt.Errorf("unable to create load balancer. err [%v]", err)
		}
		if isGatewayUsingIpSpaces {
			klog.Infof("Determined gateway [%s] is using IP spaces, using IP space specific logic to reserve an IP", gm.GatewayRef.Name)
			externalIP, err = gm.ReserveIpForLoadBalancer(ctx, lbIpClaimMarker)
			if err != nil {
				return "", fmt.Errorf("unable to reservce IP address for load balancer. error [%v]", err)
			}
		} else {
			klog.Infof("Determined gateway [%s] is not using IP spaces, using legacy IPAM solution to find a free IP", gm.GatewayRef.Name)
			externalIP, err = gm.GetUnusedExternalIPAddress(ctx, gm.IPAMSubnet)
			if err != nil {
				return "", fmt.Errorf("unable to get unused IP address from subnet [%s]: [%v]",
					gm.IPAMSubnet, err)
			}
		}
	}
	klog.Infof("Using VIP [%s] for virtual service\n", externalIP)

	for _, portDetails := range portDetailsList {
		if portDetails.InternalPort == 0 {
			klog.Infof("No internal port specified for [%s], hence loadbalancer not created\n",
				portDetails.PortSuffix)
			continue
		}

		virtualServiceName := fmt.Sprintf("%s-%s", virtualServiceNamePrefix, portDetails.PortSuffix)
		lbPoolName := fmt.Sprintf("%s-%s", lbPoolNamePrefix, portDetails.PortSuffix)

		vsSummary, err := gm.GetVirtualService(ctx, virtualServiceName)
		if err != nil {
			return "", fmt.Errorf("unexpected error while querying for virtual service [%s]: [%v]",
				virtualServiceName, err)
		}
		if vsSummary != nil {
			if vsSummary.LoadBalancerPoolRef.Name != lbPoolName {
				return "", fmt.Errorf("virtual Service [%s] found with unexpected loadbalancer pool [%s]",
					virtualServiceName, lbPoolName)
			}

			klog.V(3).Infof("LoadBalancer Virtual Service [%s] already exists", virtualServiceName)
			resourcesAllocated.Insert(VcdResourceVirtualService, &swaggerClient.EntityReference{
				Name: vsSummary.Name,
				Id:   vsSummary.Id,
			})

			if err = gm.CheckIfVirtualServiceIsPending(ctx, virtualServiceName); err != nil {
				return "", err
			}

			continue
		}

		virtualServiceIP := externalIP
		if oneArm != nil {
			internalIP := ""
			if enableVirtualServiceSharedIP {
				// a new feature in VCD >= 10.4 allows virtual services to be
				// created with the same IP and different ports
				internalIP = sharedInternalIP
			} else {
				internalIP, err = gm.GetUnusedInternalIPAddress(ctx, oneArm)
				if err != nil {
					return "", fmt.Errorf("unable to get internal IP address for one-arm mode: [%v]", err)
				}
			}

			dnatRuleName := GetDNATRuleName(virtualServiceName)

			// create app port profile
			appPortProfileName := GetAppPortProfileName(dnatRuleName)
			appPortProfile, err := gm.CreateAppPortProfile(appPortProfileName, portDetails.ExternalPort)
			if err != nil {
				return "", fmt.Errorf("failed to create App Port Profile: [%v]", err)
			}
			if appPortProfile == nil || appPortProfile.NsxtAppPortProfile == nil {
				return "", fmt.Errorf("creation of app port profile succeeded but app port profile is empty")
			}
			resourcesAllocated.Insert(VcdResourceAppPortProfile, &swaggerClient.EntityReference{
				Name: appPortProfile.NsxtAppPortProfile.Name,
				Id:   appPortProfile.NsxtAppPortProfile.ID,
			})

			if err = gm.CreateDNATRule(ctx, dnatRuleName, externalIP, internalIP,
				portDetails.ExternalPort, portDetails.InternalPort, appPortProfile); err != nil {
				return "", fmt.Errorf("unable to create dnat rule [%s:%d]=>[%s:%d] with profile [%v]: [%v]",
					externalIP, portDetails.ExternalPort, internalIP, portDetails.InternalPort, appPortProfile, err)
			}
			resourcesAllocated.Insert(VcdResourceDNATRule, &swaggerClient.EntityReference{
				Name: dnatRuleName,
			})

			// use the internal IP to create virtual service
			virtualServiceIP = internalIP

			// We get an IP address above and try to get-or-create a DNAT rule from external IP => internal IP.
			// If the rule already existed, the old DNAT rule will remain unchanged. Hence, we get the old externalIP
			// from the old rule and use it. What happens to the new externalIP that we selected above? It just remains
			// unused and hence does not get allocated and disappears. Since there is no IPAM based resource
			// _acquisition_, the new externalIP can just be forgotten about.
			dnatRuleRef, err := gm.GetNATRuleRef(ctx, dnatRuleName)
			if err != nil {
				return "", fmt.Errorf("unable to retrieve created dnat rule [%s]: [%v]", dnatRuleName, err)
			}
			if dnatRuleRef == nil {
				return "", fmt.Errorf("retrieved dnat rule ref is nil")
			}
			resourcesAllocated.Insert(VcdResourceDNATRule, &swaggerClient.EntityReference{
				Name: dnatRuleRef.Name,
				Id:   dnatRuleRef.ID,
			})

			externalIP = dnatRuleRef.ExternalIP
		} else if oneArm == nil && enableVirtualServiceSharedIP { // use external ip for virtual service
			// no dnat rule is needed because there is a feature in VCD >= 10.4
			// in which multiple virtual services can be created with the same IP and different ports
			virtualServiceIP = externalIP
		}

		segRef, err := gm.GetLoadBalancerSEG(ctx)
		if err != nil {
			return "", fmt.Errorf("unable to get service engine group from edge [%s]: [%v]",
				gm.GatewayRef.Name, err)
		}

		lbPoolRef, err := gm.CreateLoadBalancerPool(ctx, lbPoolName, ips, portDetails.InternalPort,
			portDetails.Protocol)
		if err != nil {
			return "", fmt.Errorf("unable to create load balancer pool [%s]: [%v]", lbPoolName, err)
		}
		resourcesAllocated.Insert(VcdResourceLoadBalancerPool, lbPoolRef)

		virtualServiceRef, err := gm.CreateVirtualService(ctx, virtualServiceName, lbPoolRef, segRef,
			virtualServiceIP, portDetails.Protocol, portDetails.ExternalPort,
			portDetails.UseSSL, portDetails.CertAlias)
		if err != nil {
			// return  plain error if vcdsdk.VirtualServicePendingError is returned. Helps the caller recognize that the
			// error is because VirtualService is still in Pending state.
			if _, ok := err.(*VirtualServicePendingError); ok {
				resourcesAllocated.Insert(VcdResourceVirtualService, virtualServiceRef)
				klog.Infof("Load Balancer with virtual service [%v], pool [%v] on gateway [%s] is pending\n",
					virtualServiceRef, lbPoolRef, gm.GatewayRef.Name)
				continue
			}
			return "", err
		}
		resourcesAllocated.Insert(VcdResourceVirtualService, virtualServiceRef)

		klog.Infof("Created Load Balancer with virtual service [%v], pool [%v] on gateway [%s]\n",
			virtualServiceRef, lbPoolRef, gm.GatewayRef.Name)
	}

	resourcesAllocated.Insert("externalIP", &swaggerClient.EntityReference{
		Name: externalIP,
	})

	return externalIP, nil
}

func (gm *GatewayManager) DeleteLoadBalancer(
	ctx context.Context, virtualServiceNamePrefix string, lbPoolNamePrefix string, lbIpClaimMarker string,
	portDetailsList []PortDetails, oneArm *OneArm, resourcesDeallocated *util.AllocatedResourcesMap) (string, error) {

	if gm == nil {
		return "", fmt.Errorf("GatewayManager cannot be nil")
	}

	client := gm.Client
	client.RWLock.Lock()
	defer client.RWLock.Unlock()

	// TODO: try to continue in case of errors
	var err error

	// Here the principle is to delete what is available; retry in case of failure
	// but do not fail for missing entities, since a retry will always have missing
	// entities.
	rdeVIP := ""
	for _, portDetails := range portDetailsList {
		if portDetails.InternalPort == 0 {
			klog.Infof("No internal port specified for [%s], hence loadbalancer not created\n",
				portDetails.PortSuffix)
			continue
		}

		virtualServiceName := fmt.Sprintf("%s-%s", virtualServiceNamePrefix, portDetails.PortSuffix)
		lbPoolName := fmt.Sprintf("%s-%s", lbPoolNamePrefix, portDetails.PortSuffix)

		// get external IP
		// it is weird that we are computing the same external Id multiple times in the loop
		dnatRuleName := ""
		if oneArm != nil {
			dnatRuleName = GetDNATRuleName(virtualServiceName)
			dnatRuleRef, err := gm.GetNATRuleRef(ctx, dnatRuleName)
			if err != nil {
				return "", fmt.Errorf("unable to get dnat rule ref for nat rule [%s]: [%v]", dnatRuleName, err)
			}
			if dnatRuleRef != nil {
				rdeVIP = dnatRuleRef.ExternalIP
			}
		} else {
			vsSummary, err := gm.GetVirtualService(ctx, virtualServiceName)
			if err != nil {
				return "", fmt.Errorf("unable to get summary for LB Virtual Service [%s]: [%v]",
					virtualServiceName, err)
			}
			if vsSummary != nil {
				rdeVIP = vsSummary.VirtualIpAddress
			}
		}

		err = gm.DeleteVirtualService(ctx, virtualServiceName, false)
		if err != nil {
			if vsBusyErr, ok := err.(*VirtualServiceBusyError); ok {
				klog.Errorf("delete virtual service failed; virtual service [%s] is busy: [%v]",
					virtualServiceName, err)
				return "", vsBusyErr
			}
			return "", fmt.Errorf("unable to delete virtual service [%s]: [%v]", virtualServiceName, err)
		}
		// removal from vCDResourceSet is based on ID and type comparison.
		virtualServiceRef := &swaggerClient.EntityReference{
			Name: virtualServiceName,
		}
		resourcesDeallocated.Insert(VcdResourceVirtualService, virtualServiceRef)

		err = gm.DeleteLoadBalancerPool(ctx, lbPoolName, false)
		if err != nil {
			if lbPoolBusyErr, ok := err.(*LoadBalancerPoolBusyError); ok {
				klog.Errorf("delete loadbalancer pool failed; loadbalancer pool [%s] is busy: [%v]", lbPoolName, err)
				return "", lbPoolBusyErr
			}
			return "", fmt.Errorf("unable to delete load balancer pool [%s]: [%v]", lbPoolName, err)
		}
		resourcesDeallocated.Insert(VcdResourceLoadBalancerPool, &swaggerClient.EntityReference{
			Name: lbPoolName,
		})

		if oneArm != nil {
			err = gm.DeleteDNATRule(ctx, dnatRuleName, false)
			if err != nil {
				return "", fmt.Errorf("unable to delete dnat rule [%s]: [%v]", dnatRuleName, err)
			}
			resourcesDeallocated.Insert(VcdResourceDNATRule, &swaggerClient.EntityReference{
				Name: dnatRuleName,
			})
			appPortProfileName := GetAppPortProfileName(dnatRuleName)
			err = gm.DeleteAppPortProfile(appPortProfileName, false)
			if err != nil {
				return "", fmt.Errorf("unable to delete app port profile [%s]: [%v]", appPortProfileName, err)
			}
			resourcesDeallocated.Insert(VcdResourceAppPortProfile, &swaggerClient.EntityReference{
				Name: appPortProfileName,
			})
		}
	}

	isGatewayUsingIpSpaces, err := gm.IsUsingIpSpaces()
	if err != nil {
		return "", fmt.Errorf("unable to release IP [%s] used by load balancer. err [%v]", rdeVIP, err)
	}
	if isGatewayUsingIpSpaces {
		klog.Infof("Determined gateway [%s] is using IP spaces, using IP space specific logic to release IP [%s]", gm.GatewayRef.Name, rdeVIP)
		err = gm.ReleaseIpFromLoadBalancer(ctx, rdeVIP, lbIpClaimMarker)
		if err != nil {
			return "", fmt.Errorf("unable to release IP address [%s] from load balancer. error [%v]", rdeVIP, err)
		}
	}
	// if gateway is using IP blocks, no need to explicitly release the IP

	return rdeVIP, nil
}

func (gm *GatewayManager) UpdateLoadBalancer(ctx context.Context, lbPoolName string, virtualServiceName string,
	ips []string, externalIP string, internalPort int32, externalPort int32, oneArm *OneArm, enableVirtualServiceSharedIP bool, protocol string,
	resourcesAllocated *util.AllocatedResourcesMap) (string, error) {

	if gm == nil {
		return "", fmt.Errorf("GatewayManager cannot be nil")
	}

	client := gm.Client
	client.RWLock.Lock()
	defer client.RWLock.Unlock()

	lbPoolRef, err := gm.UpdateLoadBalancerPool(ctx, lbPoolName, ips, internalPort, protocol)
	if err != nil {
		if lbPoolBusyErr, ok := err.(*LoadBalancerPoolBusyError); ok {
			klog.Errorf("update loadbalancer pool failed; loadbalancer pool [%s] is busy: [%v]", lbPoolName, err)
			return "", lbPoolBusyErr
		}
		return "", fmt.Errorf("unable to update load balancer pool [%s]: [%v]", lbPoolName, err)
	}
	resourcesAllocated.Insert(VcdResourceLoadBalancerPool, lbPoolRef)
	vsRef, err := gm.UpdateVirtualService(ctx, virtualServiceName, externalIP, externalPort, oneArm != nil)
	if vsRef != nil {
		resourcesAllocated.Insert(VcdResourceVirtualService, vsRef)
	}
	if err != nil {
		if vsBusyErr, ok := err.(*VirtualServiceBusyError); ok {
			klog.Errorf("update virtual service failed; virtual service [%s] is busy: [%v]", virtualServiceName, err)
			return "", vsBusyErr
		}
		return "", fmt.Errorf("unable to update virtual service [%s] with port [%d]: [%v]", virtualServiceName, externalPort, err)
	}
	// The condition enableVirtualServiceSharedIP == nil, oneArm == nil is not a valid configuration.
	// ValidateCloudConfig returns an error if CPI has the above configuration
	if !(enableVirtualServiceSharedIP && oneArm == nil) { // dnat not used if vsSharedIP is used and oneArm is nil
		// update app port profile
		// cases handled here -
		// enableVirtualServiceSharedIP = true, oneArm != nil
		// enableVirtualServiceSharedIP = false, oneArm == nil -> an error case which is handled earlier
		// enableVirtualServiceSharedIP = false, oneArm != nil
		dnatRuleName := GetDNATRuleName(virtualServiceName)
		appPortProfileName := GetAppPortProfileName(dnatRuleName)
		// while updating the app port profile, if we find that the app port profile with the given name is not found,
		// we silently ignore the error. This is because of an issue with CAPVCD where CAPVCD v0.5.x clusters did not
		// create app port profiles for the DNAT rules.
		// The check for govcd.ErrorEntityNotFound should be removed when we remove oneArm support.
		appPortProfileRef, err := gm.UpdateAppPortProfile(appPortProfileName, externalPort)
		if err == nil {
			if appPortProfileRef != nil && appPortProfileRef.NsxtAppPortProfile != nil {
				resourcesAllocated.Insert(VcdResourceAppPortProfile, &swaggerClient.EntityReference{
					Name: appPortProfileName,
					Id:   appPortProfileRef.NsxtAppPortProfile.ID,
				})
			}
		} else if err == govcd.ErrorEntityNotFound {
			klog.V(4).Infof("application port profile with the name [%s] is not found. skipping updating the application port profile", appPortProfileName)
		} else {
			// err != nil && err != govcd.ErrorEntityNotFound
			return "", fmt.Errorf("unable to update application port profile [%s] with external port [%d]: [%v]", appPortProfileName, externalPort, err)
		}

		// update DNAT rule
		dnatRuleRef, err := gm.GetNATRuleRef(ctx, dnatRuleName)
		if err != nil {
			return "", fmt.Errorf("unable to retrieve created dnat rule [%s]: [%v]", dnatRuleName, err)
		}
		dnatExternalIP := externalIP
		if dnatExternalIP == "" {
			dnatExternalIP = dnatRuleRef.ExternalIP
		}
		updatedDnatRule, err := gm.UpdateDNATRule(ctx, dnatRuleName, dnatExternalIP, dnatRuleRef.InternalIP, externalPort)
		if err != nil {
			return "", fmt.Errorf("unable to update DNAT rule [%s]: [%v]", dnatRuleName, err)
		}
		resourcesAllocated.Insert(VcdResourceDNATRule, &swaggerClient.EntityReference{
			Name: dnatRuleName,
			Id:   dnatRuleRef.ID,
		})
		return updatedDnatRule.ExternalIP, nil
	}
	// handle cases -
	// enableVirtualServiceSharedIP == true && oneArm == nil
	vsSummary, err := gm.GetVirtualService(ctx, virtualServiceName)
	if err != nil {
		return "", fmt.Errorf("failed to get virtual service summary for the virtual service with name [%s]", virtualServiceName)
	}
	return vsSummary.VirtualIpAddress, nil
}

// FetchIpSpacesBackingGateway Fetch list of Ip Spaces (Id) accessible to the gateway
// If gateway is not using Ip Spaces, error would be generated that will contain the underlying VCD 403 error.
func (gm *GatewayManager) FetchIpSpacesBackingGateway(ctx context.Context) ([]string, error) {
	ipSpaceService := gm.Client.APIClient.IpSpacesApi

	filterString := fmt.Sprintf("gatewayId==%s", gm.GatewayRef.Id)
	options := swaggerClient.IpSpacesApiGetFloatingIpSuggestionsOpts{Filter: optional.NewString(filterString), SortAsc: optional.NewString("ipSpaceRef.name"), SortDesc: optional.EmptyString()}
	var pageSize int32 = 10
	var pageNum int32 = 1
	var resultTotal int32 = math.MaxInt32

	ipSpaceIds := make([]string, 0)
	for int32(math.Ceil(float64(resultTotal)/float64(pageSize))) >= pageNum {
		suggestions, _, err := ipSpaceService.GetFloatingIpSuggestions(ctx, pageNum, pageSize, &options)
		if err != nil {
			return nil, fmt.Errorf("unable to get floating IP suggestion for gateway [%s] : [%v]", gm.GatewayRef.Name, err)
		}
		resultTotal = suggestions.ResultTotal

		// The way swagger client works, it is guaranteed that suggestions.Values will never be nil,
		// at worst it will be [], in which case we should bail out and not try to fetch more pages.
		if len(suggestions.Values) == 0 {
			break
		}
		for _, element := range suggestions.Values {
			ipSpaceRef := element.IpSpaceRef
			if ipSpaceRef != nil {
				ipSpaceIds = append(ipSpaceIds, ipSpaceRef.Id)
			}
		}

		pageNum += 1
	}

	return ipSpaceIds, nil
}

// FilterIpSpacesByType Takes in a list of Ip Space Ids and returns a list of govcd.IpSpace pointers that match the
// filter value of "type". Valid value for "type" = ["PUBLIC" (types.IpSpacePublic), "PRIVATE" (types.IpSpacePrivate)]
func (gm *GatewayManager) FilterIpSpacesByType(ipSpaceIds []string, ipSpaceType string) ([]*govcd.IpSpace, error) {
	if (ipSpaceType != types.IpSpacePublic) && (ipSpaceType != types.IpSpacePrivate) {
		return nil, fmt.Errorf("invalid type [%s] specified, expecting value from [PUBLIC, PRIVATE]", ipSpaceType)
	}
	if ipSpaceIds == nil {
		ipSpaceIds = make([]string, 0)
	}

	filteredIpSpaces := make([]*govcd.IpSpace, 0)
	for _, ipSpaceId := range ipSpaceIds {
		ipSpace, err := gm.Client.VCDClient.GetIpSpaceById(ipSpaceId)
		if err != nil {
			return nil, fmt.Errorf("unable to fetch details of Ip Space with id [%s], error [%v]", ipSpaceId, err)
		}
		if ipSpace.IpSpace.Type == ipSpaceType {
			filteredIpSpaces = append(filteredIpSpaces, ipSpace)
		}
	}

	return filteredIpSpaces, nil
}

// AllocateIpFromIpSpace Allocates a floating IP from a given Ip Space
// returns the allocation Id, and allocated Ip on success
// Note : the return types are string because there is no way to convert
// IpSpaceIpAllocationRequestResult (output of org.IpSpaceAllocateIp) into an IpSpaceIpAllocation object
func (gm *GatewayManager) AllocateIpFromIpSpace(ipSpace *govcd.IpSpace) (string, string, error) {
	if ipSpace == nil {
		return "", "", fmt.Errorf("unable to allocate floating Ip from Ip Space [nil]")
	}

	org, err := gm.Client.VCDClient.GetOrgByName(gm.Client.ClusterOrgName)
	if err != nil {
		return "", "", fmt.Errorf("unable to allocate floating Ip from Ip Space [%s]. error [%v]", ipSpace.IpSpace.Name, err)
	}

	val := 1
	request := types.IpSpaceIpAllocationRequest{
		Type:     types.IpSpaceIpAllocationTypeFloatingIp,
		Quantity: &val,
	}
	result, err := org.IpSpaceAllocateIp(ipSpace.IpSpace.ID, &request)

	if err != nil {
		return "", "", fmt.Errorf("unable to allocate floating IP from Ip Space [%s]. error [%v]", ipSpace.IpSpace.Name, err)
	}

	if result == nil || len(result) != 1 {
		return "", "", fmt.Errorf("unable to allocate exactly 1 floating Ip from Ip Space [%s]", ipSpace.IpSpace.Name)
	}

	return result[0].ID, result[0].Value, nil
}

// FindIpAllocationByIp Finds an IP allocation in the given Ip Space that matches the provided IP address
// returns no error if no such allocation is found
func (gm *GatewayManager) FindIpAllocationByIp(ipSpace *govcd.IpSpace, allocatedIp string) (*govcd.IpSpaceIpAllocation, error) {
	if ipSpace == nil {
		return nil, fmt.Errorf("unable to find allocation for floating Ip [%s] in Ip Space [nil]", allocatedIp)
	}
	queryParams := url.Values{}
	queryParams.Set("filter", fmt.Sprintf("value==%s", allocatedIp))

	allocations, err := ipSpace.GetAllIpSpaceAllocations(types.IpSpaceIpAllocationTypeFloatingIp, queryParams)
	if err != nil {
		return nil, fmt.Errorf("unable to find allocation in Ip Space [%s], correspnding to Ip [%s]. error [%v]", ipSpace.IpSpace.Name, allocatedIp, err)
	}

	// If no allocations were made on the Ip Space with the specified Ip, it is valid to return no result with no error
	// This use case is important for us, hence skipping GetIpSpaceAllocationByTypeAndValue
	if allocations == nil || len(allocations) == 0 {
		return nil, nil
	}

	if len(allocations) == 1 {
		return allocations[0], nil
	}

	// Ideally we should never reach here, since VCD shouldn't make multiple allocations for the same IP
	return nil, fmt.Errorf("found multiple allocations in Ip Space [%s] for Ip [%s]", ipSpace.IpSpace.Name, allocatedIp)
}

// FindIpAllocationByMarker We are adding info in the description of IpAllocation
// to record that particular allocation was made by a CSE cluster. This method finds an
// allocation corresponding to the previously stored description
func (gm *GatewayManager) FindIpAllocationByMarker(ipSpace *govcd.IpSpace, marker string) (*govcd.IpSpaceIpAllocation, error) {
	if ipSpace == nil {
		return nil, fmt.Errorf("unable to find allocation for marker [%s] in Ip Space [nil]", marker)
	}
	queryParams := url.Values{}
	queryParams.Set("filter", fmt.Sprintf("usageState==%s", types.IpSpaceIpAllocationUsedManual))

	allocations, err := ipSpace.GetAllIpSpaceAllocations(types.IpSpaceIpAllocationTypeFloatingIp, queryParams)
	if err != nil {
		return nil, fmt.Errorf("unable to find allocation in Ip Space [%s], correspnding to marker [%s]. error [%v]", ipSpace.IpSpace.Name, marker, err)
	}

	// If no allocations were made on the Ip Space, it is valid to return no result without error
	if allocations == nil || len(allocations) == 0 {
		return nil, nil
	}

	for _, allocation := range allocations {
		if allocation.IpSpaceIpAllocation.Description == marker {
			return allocation, nil
		}
	}

	// If no allocation with the marker is found it is ok to return back with no result and no error
	return nil, nil
}

// MarkIpAsUsed We are adding info in the description  of IpAllocation
// to track that particular allocation was made by CSE cluster. This method updates
// the state and description of an allocation to mark the cluster which owns the allocation.
func (gm *GatewayManager) MarkIpAsUsed(ipSpaceAllocation *govcd.IpSpaceIpAllocation, marker string) (*govcd.IpSpaceIpAllocation, error) {
	if ipSpaceAllocation == nil {
		return nil, fmt.Errorf("unable to mark Ip Allocation [nil] as used, via marker [%s]", marker)
	}
	updateConfig := ipSpaceAllocation.IpSpaceIpAllocation
	updateConfig.UsageState = types.IpSpaceIpAllocationUsedManual
	updateConfig.Description = marker

	return ipSpaceAllocation.Update(updateConfig)
}

// MarkIpAsUnused We are adding info in the description  of IpAllocation
// to track that particular allocation was made by CSE cluster. This method updates
// the state and description of an allocation to mark that the cluster no longer owns the allocation.
func (gm *GatewayManager) MarkIpAsUnused(ipSpaceAllocation *govcd.IpSpaceIpAllocation) (*govcd.IpSpaceIpAllocation, error) {
	if ipSpaceAllocation == nil {
		return nil, fmt.Errorf("unable to mark IP allocation [nil] as unused")
	}
	updateConfig := ipSpaceAllocation.IpSpaceIpAllocation
	updateConfig.UsageState = types.IpSpaceIpAllocationUnused
	updateConfig.Description = ""

	return ipSpaceAllocation.Update(updateConfig)
}

func (gm *GatewayManager) ReleaseIp(ipSpaceAllocation *govcd.IpSpaceIpAllocation) error {
	if ipSpaceAllocation == nil {
		return fmt.Errorf("unable to release Ip Allocation [nil]")
	}
	return ipSpaceAllocation.Delete()
}

// ReserveIpForLoadBalancer will scan through all Ip Spaces available to the gateway for an existing allocation
// (description of allocation will contain cluster id, service name and namespace). If such an allocation can't be
// retrieved, a new Ip Allocation will be attempted against all Ip Spaces, sequentially. The first Ip Space that allows
// the reservation to go through, will conclude the process. The allocation will be moved to USED_MANUAL state and its
// description will be updated to mark a claim. The allocated Ip will be returned. If all Ip Spaces reject the
// allocation request, this method will return an error
func (gm *GatewayManager) ReserveIpForLoadBalancer(ctx context.Context, claimMarker string) (string, error) {
	ipSpaceIds, err := gm.FetchIpSpacesBackingGateway(ctx)
	if err != nil {
		return "", fmt.Errorf("unable to reserve IP from Ip Space. error [%v]", err)
	}

	publicIpSpaces, err := gm.FilterIpSpacesByType(ipSpaceIds, types.IpSpacePublic)
	if err != nil {
		return "", fmt.Errorf("unable to reserve IP from Ip Space. error [%v]", err)
	}

	for _, ipSpace := range publicIpSpaces {
		ipSpaceAllocation, err := gm.FindIpAllocationByMarker(ipSpace, claimMarker)
		if err != nil {
			return "", fmt.Errorf("unable to reserve IP from Ip Space [%s]. error [%v]", ipSpace.IpSpace.Name, err)
		}
		// Found an existing allocation for this particular service
		if ipSpaceAllocation != nil {
			return ipSpaceAllocation.IpSpaceIpAllocation.Value, nil
		}
	}

	// if we haven't found any allocation yet on all accessible Ip Spaces, we need to create a new allocation
	// NOTE: The allocation mechanism needs two calls to VCD and should be treated like a critical section
	// Under all circumstances, two instances of CPI will never try to create a lb for a service simultaneously, so we should be good.
	for _, ipSpace := range publicIpSpaces {
		_, allocatedIp, err := gm.AllocateIpFromIpSpace(ipSpace)
		if err != nil {
			// don't give up yet, allocation can fail because Ip Space has no free Ip, try the next Ip Space
			klog.Infof("unable to reserve IP from Ip Space [%s]. error [%v]. will try next ip Space.", ipSpace.IpSpace.Name, err)
			continue
		}

		// We were able to make an allocation, so we should be able to retrieve it
		// if retrieval fails, we should fail and not try to allocate another IP from the next
		// IP space
		ipSpaceAllocation, err := gm.FindIpAllocationByIp(ipSpace, allocatedIp)
		if err != nil || ipSpaceAllocation == nil {
			klog.Infof("leaked IP [%s] from Ip Space [%s]. Unable to retrieve allocated IP.", allocatedIp, ipSpace.IpSpace.Name)
			return "", fmt.Errorf("unable to reserve IP from Ip Space [%s]. error [%v]", ipSpace.IpSpace.Name, err)
		}

		_, err = gm.MarkIpAsUsed(ipSpaceAllocation, claimMarker)
		if err != nil {
			klog.Infof("leaked IP [%s] from Ip Space [%s]. Unable to mark allocated IP as used.", allocatedIp, ipSpace.IpSpace.Name)
			return "", fmt.Errorf("unable to reserve IP from Ip Space [%s]. error [%v]", ipSpace.IpSpace.Name, err)
		}
		return ipSpaceAllocation.IpSpaceIpAllocation.Value, nil
	}

	// Was unable to reserve an Ip on any of the available Ip Spaces
	return "", fmt.Errorf("unable to reserve Ip from any available Ip spaces")
}

// ReleaseIpFromLoadBalancer will scan through all Ip Spaces available to the gateway for an existing allocation
// (description of allocation will contain cluster id, service name and namespace). If such an allocation can't be
// retrieved, the method will return without raising any error. It should be assumed that the allocation was removed in
// a previous attempt. If an allocation is found, it will be deleted, thereby releasing the IP from the load balancer as well
// as tenant context. It should be noted that if the cluster was created with user provider external IP, then the allocation
// will not be present on any of the IP Spaces, and hence we will not try to release the IP.
func (gm *GatewayManager) ReleaseIpFromLoadBalancer(ctx context.Context, rdeVIP string, claimMarker string) error {
	ipSpaceIds, err := gm.FetchIpSpacesBackingGateway(ctx)
	if err != nil {
		return fmt.Errorf("unable to release IP [%s] from load balancer. error [%v]", rdeVIP, err)
	}

	publicIpSpaces, err := gm.FilterIpSpacesByType(ipSpaceIds, types.IpSpacePublic)
	if err != nil {
		return fmt.Errorf("unable to release IP [%s] from load balancer. error [%v]", rdeVIP, err)
	}

	for _, ipSpace := range publicIpSpaces {
		ipSpaceAllocation, err := gm.FindIpAllocationByMarker(ipSpace, claimMarker)
		if err != nil {
			return fmt.Errorf("unable to release IP [%s] from Ip Space [%s]. error [%v]", rdeVIP, ipSpace.IpSpace.Name, err)
		}
		// In case an allocation is not found on this Ip Space, we will have ipSpaceAllocation = nil, err = nil
		if ipSpaceAllocation != nil {
			// Found an existing allocation for this particular service
			allocatedIp := ipSpaceAllocation.IpSpaceIpAllocation.Value
			if allocatedIp != rdeVIP {
				return fmt.Errorf("RDE VIP [%s] doesn't match allocated IP [%s] in IP Space [%s] for marker [%s]", rdeVIP, allocatedIp, ipSpace.IpSpace.Name, claimMarker)
			}
			updatedIpSpaceAllocation, err := gm.MarkIpAsUnused(ipSpaceAllocation)
			if err != nil {
				return fmt.Errorf("unable to mark IP [%s] from IP Space [%s] as unused. error [%v]", allocatedIp, ipSpace.IpSpace.Name, err)
			}
			err = gm.ReleaseIp(updatedIpSpaceAllocation)
			if err != nil {
				return fmt.Errorf("unable to release IP [%s] from IP Space [%s]. error [%v]", allocatedIp, ipSpace.IpSpace.Name, err)
			}
			// No need to process any more IP spaces, it is safe to return from this function
			return nil
		}
	}

	// If we have reached here, it means, we couldn't locate an allocation corresponding to the marker
	// Either the IP was released in a previous attempt, or the external IP was assigned manually by
	// the user. In either case, we have nothing to do here and we should safely return.
	return nil
}
