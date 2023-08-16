/*
   Copyright 2021 VMware, Inc.
   SPDX-License-Identifier: Apache-2.0
*/

package util

import (
	"bytes"
	"context"
	"encoding/json"
	"encoding/xml"
	"fmt"
	"github.com/vmware/cloud-provider-for-cloud-director/pkg/vcdsdk"
	swagger "github.com/vmware/cloud-provider-for-cloud-director/pkg/vcdswaggerclient_36_0"
	"github.com/vmware/go-vcloud-director/v2/govcd"
	"github.com/vmware/go-vcloud-director/v2/types/v56"
	"github.com/vmware/go-vcloud-director/v2/util"
	"io/ioutil"
	"net/http"
	"strings"
)

// indentJsonBody indents raw JSON body for easier readability
func indentJsonBody(body []byte) ([]byte, error) {
	var prettyJSON bytes.Buffer
	err := json.Indent(&prettyJSON, body, "", "  ")
	if err != nil {
		return nil, fmt.Errorf("error indenting response JSON: %s", err)
	}
	body = prettyJSON.Bytes()
	return body, nil
}

// DecodeXMLBody is used to decode a response body of types.BodyType
func DecodeXMLBody(bodyType types.BodyType, resp *http.Response, out interface{}) error {
	body, err := ioutil.ReadAll(resp.Body)

	// In case of JSON, body does not have indents in response therefore it must be indented
	if bodyType == types.BodyTypeJSON {
		body, err = indentJsonBody(body)
		if err != nil {
			return err
		}
	}

	util.ProcessResponseOutput(util.FuncNameCallStack(), resp, fmt.Sprintf("%s", body))
	if err != nil {
		return err
	}

	// only attempt to unmarshal if body is not empty
	if len(body) > 0 {
		switch bodyType {
		case types.BodyTypeXML:
			if err = xml.Unmarshal(body, &out); err != nil {
				return err
			}
		case types.BodyTypeJSON:
			if err = json.Unmarshal(body, &out); err != nil {
				return err
			}

		default:
			panic(fmt.Sprintf("unknown body type: %d", bodyType))
		}
	}

	return nil
}

func Bool2BoolPtr(val bool) *bool {
	return &val
}

func Int2IntPtr(val int) *int {
	return &val
}

func Float2FloatPtr(val float64) *float64 {
	return &val
}

// Str2Bool returns true if the string value is not false
func Str2Bool(val string) bool {
	return strings.ToLower(val) == "true"
}

type EdgeGatewayDetails struct {
	OvdcNetworkReference *swagger.EntityReference
	Vdc                  *govcd.Vdc
	EdgeGatewayReference *swagger.EntityReference
	EndPointHost         string
	EndPointPort         int
}

func GetOVDCFromOVDCName(vcdClient *govcd.VCDClient, orgName string, ovdcName string) (*govcd.Vdc, error) {
	org, err := vcdClient.GetOrgByName(orgName)
	if err != nil {
		return nil, fmt.Errorf("unable to get org from name [%s]: [%v]", orgName, err)
	}

	vdc, err := org.GetVDCByName(ovdcName, true)
	if err != nil {
		return nil, fmt.Errorf("unable to get OVDC [%s] from org [%s]: [%v]", ovdcName, orgName, err)
	}

	return vdc, nil
}

// GetOVDCNetwork AMK:TODO is borrowed from gatewayManager.getOVDCNetwork with some minor changes since the former
// is not exported. This is tech debt and needs to be cleaned up
func GetOVDCNetwork(ctx context.Context, client *vcdsdk.Client,
	ovdcNetworkName string, ovdcName string) (*swagger.VdcNetwork, error) {

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
			if ovdcNetwork.Name == ovdcNetworkName && (ovdcNetwork.OrgVdc == nil || ovdcNetwork.OrgVdc.Name == ovdcName) {
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

func GetAllDetailsForOVDC(ctx context.Context, client *vcdsdk.Client, vdc *govcd.Vdc, ovdcNetworkName string,
	ovdcName string, orgName string) (*swagger.VdcNetwork, *govcd.Vdc, error) {

	var err error
	if vdc == nil {
		vdc, err = GetOVDCFromOVDCName(client.VCDClient, orgName, ovdcName)
		if err != nil {
			return nil, nil, fmt.Errorf(
				"unable to get VDC for Org [%s], OVDC Name [%s]: [%v]", orgName, ovdcName, err)
		}
	}
	if vdc.Vdc == nil {
		return nil, nil, fmt.Errorf("nil Vdc object in vdc")
	}

	ovdcNetwork, err := GetOVDCNetwork(ctx, client, ovdcNetworkName, ovdcName)
	if err != nil {
		return nil, nil, fmt.Errorf("unable to get OVDC Network [%s] in VDC [%s] and org [%s]: [%v]",
			ovdcNetworkName, ovdcName, orgName, err)
	}

	if ovdcNetwork == nil {
		return nil, nil, fmt.Errorf(
			"obtained nil ovdcNetwork for OVDC network [%s] in VDC [%s] and org [%s]: [%v]",
			ovdcNetworkName, ovdcName, orgName, err)
	}

	if ovdcNetwork.Connection == nil {
		return nil, nil, fmt.Errorf(
			"obtained nil Connection field in OVDC network [%s] in VDC [%s] and org [%s]: [%v]",
			ovdcNetworkName, ovdcName, orgName, err)
	}

	if ovdcNetwork.Connection.RouterRef == nil {
		return nil, nil, fmt.Errorf(
			"obtained nil Connection.RouterRef (gateway) field in OVDC network [%s] in VDC [%s] and org [%s]: [%v]",
			ovdcNetworkName, ovdcName, orgName, err)
	}

	return ovdcNetwork, vdc, nil
}

func CreateVAppNamePrefix(clusterName string, ovdcID string) (string, error) {
	parts := strings.Split(ovdcID, ":")
	if len(parts) != 4 {
		// urn:vcloud:org:<uuid>
		return "", fmt.Errorf("invalid URN format for OVDC: [%s]", ovdcID)
	}

	return fmt.Sprintf("%s_%s", clusterName, parts[3]), nil
}
