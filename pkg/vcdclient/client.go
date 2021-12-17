/*
   Copyright 2021 VMware, Inc.
   SPDX-License-Identifier: Apache-2.0
*/

package vcdclient

import (
	"context"
	"crypto/tls"
	"fmt"
	"k8s.io/klog"
	"net/http"
	"sync"

	swaggerClient "github.com/vmware/cluster-api-provider-cloud-director/pkg/vcdswaggerclient"
	"github.com/vmware/go-vcloud-director/v2/govcd"
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

// Client :
type Client struct {
	VcdAuthConfig          *VCDAuthConfig
	ClusterOrgName         string
	ClusterOVDCName        string
	ClusterVAppName        string
	VcdClient              *govcd.VCDClient
	Vdc                    *govcd.Vdc
	ApiClient              *swaggerClient.APIClient
	NetworkName            string
	ClusterID              string
	OneArm                 *OneArm
	HTTPPort               int32
	HTTPSPort              int32
	TCPPort                int32
	IPAMSubnet             string
	GatewayRef             *swaggerClient.EntityReference
	NetworkBackingType     swaggerClient.BackingNetworkType
	ManagementClusterRDEId string
	CertificateAlias       string
	rwLock                 sync.RWMutex
	CsiVersion             string
	CpiVersion             string
	CniVersion             string
	CAPVCDVersion          string
}

func (client *Client) RefreshBearerToken() error {
	klog.Infof("Refreshing vcd client")

	href := fmt.Sprintf("%s/api", client.VcdAuthConfig.Host)
	client.VcdClient.Client.APIVersion = VCloudApiVersion

	klog.Infof("Is user sysadmin: [%v]", client.VcdClient.Client.IsSysAdmin)
	var token string
	if client.VcdAuthConfig.RefreshToken != "" {
		// Refresh vcd client using refresh token
		accessTokenResponse, _, err := client.VcdAuthConfig.getAccessTokenFromRefreshToken(
			client.VcdClient.Client.IsSysAdmin)
		if err != nil {
			return fmt.Errorf(
				"failed to get access token from refresh token for user [%s/%s] for url [%s]: [%v]",
				client.VcdAuthConfig.UserOrg, client.VcdAuthConfig.User, href, err)
		}

		err = client.VcdClient.SetToken(client.VcdAuthConfig.UserOrg,
			"Authorization", fmt.Sprintf("Bearer %s", accessTokenResponse.AccessToken))
		if err != nil {
			return fmt.Errorf("failed to set authorization header: [%v]", err)
		}
		// The previous function call will unset IsSysAdmin boolean for administrator because govcd makes a hard check
		// on org name. Set the boolean back
		client.VcdClient.Client.IsSysAdmin = client.VcdAuthConfig.IsSysAdmin
		token = accessTokenResponse.AccessToken
	} else if client.VcdAuthConfig.User != "" && client.VcdAuthConfig.Password != "" {
		// Refresh vcd client using username and password
		resp, err := client.VcdClient.GetAuthResponse(client.VcdAuthConfig.User, client.VcdAuthConfig.Password,
			client.VcdAuthConfig.UserOrg)
		if err != nil {
			return fmt.Errorf("unable to authenticate [%s/%s] for url [%s]: [%+v] : [%v]",
				client.VcdAuthConfig.UserOrg, client.VcdAuthConfig.User, href, resp, err)
		}
		token = client.VcdClient.Client.VCDToken
	} else {
		return fmt.Errorf(
			"unable to find refresh token or secret to refresh vcd client for user [%s/%s] and url [%s]",
			client.VcdAuthConfig.UserOrg, client.VcdAuthConfig.User, href)
	}

	// reset legacy client
	org, err := client.VcdClient.GetOrgByNameOrId(client.ClusterOrgName)
	if err != nil {
		return fmt.Errorf("unable to get vcd organization [%s]: [%v]",
			client.ClusterOrgName, err)
	}

	vdc, err := org.GetVDCByName(client.ClusterOVDCName, true)
	if err != nil {
		return fmt.Errorf("unable to get vdc from org [%s], vdc [%s]: [%v]",
			client.ClusterOrgName, client.VcdAuthConfig.VDC, err)
	}
	client.Vdc = vdc

	// reset swagger client
	swaggerConfig := swaggerClient.NewConfiguration()
	swaggerConfig.BasePath = fmt.Sprintf("%s/cloudapi", client.VcdAuthConfig.Host)
	swaggerConfig.AddDefaultHeader("Authorization", fmt.Sprintf("Bearer %s", token))
	swaggerConfig.HTTPClient = &http.Client{
		Transport: &http.Transport{
			TLSClientConfig: &tls.Config{InsecureSkipVerify: client.VcdAuthConfig.Insecure},
		},
	}
	client.ApiClient = swaggerClient.NewAPIClient(swaggerConfig)

	klog.Info("successfully refreshed all clients")
	return nil
}

// NewVCDClientFromSecrets :
func NewVCDClientFromSecrets(host string, orgName string, vdcName string, vAppName string,
	networkName string, ipamSubnet string, userOrg string, user string, password string,
	refreshToken string, insecure bool, clusterID string, oneArm *OneArm, httpPort int32,
	httpsPort int32, tcpPort int32, getVdcClient bool, managementClusterRDEId string,
	csiVersion string, cpiVersion string, cniVersion string, capvcdVersion string) (*Client, error) {

	// TODO: validation of parameters

	clientCreatorLock.Lock()
	defer clientCreatorLock.Unlock()

	// Return old client if everything matches. Else create new one and cache it.
	// This is suboptimal but is not a common case.
	if clientSingleton != nil {
		if clientSingleton.VcdAuthConfig.Host == host &&
			clientSingleton.ClusterOrgName == orgName &&
			clientSingleton.ClusterOVDCName == vdcName &&
			clientSingleton.ClusterVAppName == vAppName &&
			clientSingleton.VcdAuthConfig.UserOrg == userOrg &&
			clientSingleton.VcdAuthConfig.User == user &&
			clientSingleton.VcdAuthConfig.Password == password &&
			clientSingleton.VcdAuthConfig.RefreshToken == refreshToken &&
			clientSingleton.VcdAuthConfig.Insecure == insecure &&
			clientSingleton.NetworkName == networkName &&
			clientSingleton.ManagementClusterRDEId == managementClusterRDEId &&
			clientSingleton.CsiVersion == csiVersion &&
			clientSingleton.CpiVersion == cpiVersion &&
			clientSingleton.CniVersion == cniVersion &&
			clientSingleton.CAPVCDVersion == capvcdVersion {
			return clientSingleton, nil
		}
	}

	vcdAuthConfig := NewVCDAuthConfigFromSecrets(host, user, password, refreshToken, userOrg, insecure)

	vcdClient, apiClient, err := vcdAuthConfig.GetSwaggerClientFromSecrets()
	if err != nil {
		return nil, fmt.Errorf("unable to get swagger client from secrets: [%v]", err)
	}

	client := &Client{
		VcdAuthConfig:          vcdAuthConfig,
		ClusterOrgName:         orgName,
		ClusterOVDCName:        vdcName,
		ClusterVAppName:        vAppName,
		VcdClient:              vcdClient,
		ApiClient:              apiClient,
		NetworkName:            networkName,
		IPAMSubnet:             ipamSubnet,
		GatewayRef:             nil,
		ClusterID:              clusterID,
		OneArm:                 oneArm,
		HTTPPort:               httpPort,
		HTTPSPort:              httpsPort,
		TCPPort:                tcpPort,
		ManagementClusterRDEId: managementClusterRDEId,
		CsiVersion:             csiVersion,
		CpiVersion:             cpiVersion,
		CniVersion:             cniVersion,
		CAPVCDVersion:          capvcdVersion,
	}

	if getVdcClient {
		org, err := vcdClient.GetOrgByName(orgName)
		if err != nil {
			return nil, fmt.Errorf("unable to get org from name [%s]: [%v]", orgName, err)
		}

		client.Vdc, err = org.GetVDCByName(vdcName, true)
		if err != nil {
			return nil, fmt.Errorf("unable to get vdc [%s] from org [%s]: [%v]", vdcName, orgName, err)
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

	klog.Infof("Client singleton is sysadmin: [%v]", clientSingleton.VcdClient.Client.IsSysAdmin)
	return clientSingleton, nil
}
