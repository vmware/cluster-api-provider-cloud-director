/*
   Copyright 2021 VMware, Inc.
   SPDX-License-Identifier: Apache-2.0
*/

package vcdclient

import (
	"context"
	"fmt"
	"sync"

	swaggerClient "github.com/vmware/cluster-api-provider-cloud-director/pkg/vcdswaggerclient"
	"github.com/vmware/go-vcloud-director/v2/govcd"
	"k8s.io/klog"
)

var (
	clientCreatorLock sync.Mutex
	clientSingleton   *Client = nil
)

// OneArm : internal struct representing OneArm config details
type OneArm struct {
	StartIPAddress string
	EndIPAddress   string
}

// TODO: Sahithi: Classify all the networking properties into one struct
// eg: NetworkConfig to hold networkname, ipamsubnet, gatewayref etc

// TODO: Sahithi: All api (computepolicy, gateway, vapp etc) methods are
// currently being directly associated with the Client object.
// Create separate structs for each of those api files and have them take the
// client as a param

// Client :
type Client struct {
	VcdAuthConfig      *VCDAuthConfig
	VcdClient          *govcd.VCDClient
	Vdc                *govcd.Vdc
	ApiClient          *swaggerClient.APIClient
	NetworkName        string
	ClusterID          string
	OneArm             *OneArm
	HTTPPort           int32
	HTTPSPort          int32
	TCPPort            int32
	ipamSubnet         string
	GatewayRef         *swaggerClient.EntityReference
	NetworkBackingType swaggerClient.BackingNetworkType
	rwLock             sync.RWMutex
}

// RefreshToken will check if can authenticate and rebuild clients if needed
func (client *Client) RefreshToken() error {
	_, r, err := client.VcdAuthConfig.GetBearerTokenFromSecrets()
	if r == nil && err != nil {
		return fmt.Errorf("error while getting bearer token from secrets: [%v]", err)
	} else if r != nil && r.StatusCode == 401 {
		klog.Info("Refreshing tokens as previous one has expired")
		client.VcdClient.Client.APIVersion = "35.0"
		err := client.VcdClient.Authenticate(client.VcdAuthConfig.User,
			client.VcdAuthConfig.Password, client.VcdAuthConfig.Org)
		if err != nil {
			return fmt.Errorf("unable to Authenticate user [%s]: [%v]",
				client.VcdAuthConfig.User, err)
		}

		org, err := client.VcdClient.GetOrgByNameOrId(client.VcdAuthConfig.Org)
		if err != nil {
			return fmt.Errorf("unable to get vcd organization [%s]: [%v]",
				client.VcdAuthConfig.Org, err)
		}

		vdc, err := org.GetVDCByName(client.VcdAuthConfig.VDC, true)
		if err != nil {
			return fmt.Errorf("unable to get Vdc from org [%s], Vdc [%s]: [%v]",
				client.VcdAuthConfig.Org, client.VcdAuthConfig.VDC, err)
		}

		client.Vdc = vdc
	}

	return nil
}

// NewVCDClientFromSecrets :
func NewVCDClientFromSecrets(host string, orgName string, vdcName string,
	networkName string, ipamSubnet string, user string, password string,
	insecure bool, clusterID string, oneArm *OneArm, httpPort int32,
	httpsPort int32, tcpPort int32, getVdcClient bool) (*Client, error) {

	// TODO: validation of parameters

	clientCreatorLock.Lock()
	defer clientCreatorLock.Unlock()

	// Return old client if everything matches. Else create new one and cache it.
	// This is suboptimal but is not a common case.
	if clientSingleton != nil {
		if clientSingleton.VcdAuthConfig.Host == host &&
			clientSingleton.VcdAuthConfig.Org == orgName &&
			clientSingleton.VcdAuthConfig.VDC == vdcName &&
			clientSingleton.VcdAuthConfig.User == user &&
			clientSingleton.VcdAuthConfig.Password == password &&
			clientSingleton.VcdAuthConfig.Insecure == insecure &&
			clientSingleton.NetworkName == networkName {
			return clientSingleton, nil
		}
	}

	vcdAuthConfig := NewVCDAuthConfigFromSecrets(host, user, password, orgName, insecure, vdcName)

	vcdClient, apiClient, err := vcdAuthConfig.GetSwaggerClientFromSecrets()
	if err != nil {
		return nil, fmt.Errorf("unable to get swagger client from secrets: [%v]", err)
	}

	client := &Client{
		VcdAuthConfig: vcdAuthConfig,
		VcdClient:     vcdClient,
		ApiClient:     apiClient,
		NetworkName:   networkName,
		ipamSubnet:    ipamSubnet,
		GatewayRef:    nil,
		ClusterID:     clusterID,
		OneArm:        oneArm,
		HTTPPort:      httpPort,
		HTTPSPort:     httpsPort,
		TCPPort:       tcpPort,
	}

	if getVdcClient {
		// this new client is only needed to get the Vdc pointer
		vcdClient, err = vcdAuthConfig.GetPlainClientFromSecrets()
		if err != nil {
			return nil, fmt.Errorf("unable to get plain client from secrets: [%v]", err)
		}

		org, err := vcdClient.GetOrgByName(orgName)
		if err != nil {
			return nil, fmt.Errorf("unable to get org from name [%s]: [%v]", orgName, err)
		}

		client.Vdc, err = org.GetVDCByName(vdcName, true)
		if err != nil {
			return nil, fmt.Errorf("unable to get Vdc [%s] from org [%s]: [%v]", vdcName, orgName, err)
		}
	}
	client.VcdClient = vcdClient

	// We will specifically cache the gateway ID that corresponds to the
	// network name since it is used frequently in the loadbalancer context.
	ctx := context.Background()
	gateway := &GatewayManager{
		NetworkName: networkName,
		Client:      client,
	}
	err = gateway.CacheGatewayDetails(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to cache gateway details: [%v]", err)
	}
	client.GatewayRef = gateway.GatewayRef
	client.NetworkBackingType = gateway.NetworkBackingType
	if gateway.IsNSXTBackedGateway() {
		if err = gateway.CacheGatewayDetails(ctx); err != nil {
			return nil, fmt.Errorf("unable to get gateway edge from network name [%s]: [%v]",
				client.NetworkName, err)
		}
		klog.Infof("Cached gateway details [%#v] successfully\n", client.GatewayRef)
	}
	clientSingleton = client

	return clientSingleton, nil
}
