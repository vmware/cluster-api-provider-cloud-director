/*
   Copyright 2021 VMware, Inc.
   SPDX-License-Identifier: Apache-2.0
*/

package util

import (
	"context"
	"fmt"
	"github.com/vmware/cloud-provider-for-cloud-director/pkg/vcdsdk"
	swagger "github.com/vmware/cloud-provider-for-cloud-director/pkg/vcdswaggerclient_37_2"
	"github.com/vmware/go-vcloud-director/v2/govcd"
	"strings"
)

// Str2Bool returns true if the string value is not false
func Str2Bool(val string) bool {
	return strings.ToLower(val) == "true"
}

// Keys takes a map as an input and returns a slice of keys of that map.
func Keys[M ~map[key]val, key comparable, val any](m M) []key {
	r := make([]key, len(m))
	idx := 0
	for k := range m {
		r[idx] = k
		idx = idx + 1
	}

	return r
}

type EdgeGatewayDetails struct {
	OvdcNetworkReference *swagger.EntityReference
	Vdc                  *govcd.Vdc
	EdgeGatewayReference *swagger.EntityReference
	EndPointHost         string
	EndPointPort         int
}

func GetOVDCFromOVDCIdentifier(vcdClient *govcd.VCDClient, orgName string, ovdcIdentifier string) (*govcd.Vdc, error) {
	org, err := vcdClient.GetOrgByName(orgName)
	if err != nil {
		return nil, fmt.Errorf("unable to get org from name [%s]: [%v]", orgName, err)
	}

	vdc, err := org.GetVDCByNameOrId(ovdcIdentifier, true)
	if err != nil {
		return nil, fmt.Errorf("unable to get OVDC [%s] from org [%s]: [%v]", ovdcIdentifier, orgName, err)
	}

	return vdc, nil
}

// GetOVDCNetwork :TODO is borrowed from gatewayManager.getOVDCNetwork with some minor changes since the former
// is not exported. This is tech debt and needs to be cleaned up
func GetOVDCNetwork(ctx context.Context, client *vcdsdk.Client,
	ovdcNetworkName string, ovdcIdentifier string) (*swagger.VdcNetwork, error) {

	if ovdcNetworkName == "" {
		return nil, fmt.Errorf("ovdc network name should not be empty")
	}

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
			if ovdcNetwork.Name == ovdcNetworkName &&
				(ovdcNetwork.OrgVdc == nil ||
					ovdcNetwork.OrgVdc.Name == ovdcIdentifier ||
					ovdcNetwork.OrgVdc.Id == ovdcIdentifier) {
				if networkFound {
					return nil, fmt.Errorf(
						"found more than one network with the name [%s] in the org [%s] - "+
							"please ensure the network name is unique within an org", ovdcNetworkName, client.ClusterOrgName)
				}
				ovdcNetworkID = ovdcNetwork.Id
				networkFound = true
			}
		}
		pageNum++
	}
	if ovdcNetworkID == "" {
		return nil, fmt.Errorf("unable to obtain ID for ovdc network name [%s]",
			ovdcNetworkName)
	}

	ovdcNetworkAPI := client.APIClient.OrgVdcNetworkApi
	ovdcNetwork, resp, err := ovdcNetworkAPI.GetOrgVdcNetwork(ctx, ovdcNetworkID, org.Org.ID)
	if err != nil {
		return nil, fmt.Errorf("unable to get network for id [%s]: [%+v]: [%v]", ovdcNetworkID, resp, err)
	}

	return &ovdcNetwork, nil
}

func CreateVAppNamePrefix(clusterName string, ovdcID string) (string, error) {
	parts := strings.Split(ovdcID, ":")
	if len(parts) != 4 {
		// urn:vcloud:org:<uuid>
		return "", fmt.Errorf("invalid URN format for OVDC: [%s]", ovdcID)
	}

	return fmt.Sprintf("%s_%s", clusterName, parts[3]), nil
}
