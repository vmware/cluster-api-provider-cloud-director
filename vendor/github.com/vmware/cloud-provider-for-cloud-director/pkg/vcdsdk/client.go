/*
   Copyright 2021 VMware, Inc.
   SPDX-License-Identifier: Apache-2.0
*/

package vcdsdk

import (
	"crypto/tls"
	"fmt"
	"k8s.io/klog"
	"net/http"
	"sync"

	swaggerClient "github.com/vmware/cloud-provider-for-cloud-director/pkg/vcdswaggerclient"
	"github.com/vmware/go-vcloud-director/v2/govcd"
)

var (
	clientCreatorLock sync.Mutex
	clientSingleton   *Client = nil
)

// Client :
type Client struct {
	VCDAuthConfig   *VCDAuthConfig // s
	ClusterOrgName  string
	ClusterOVDCName string
	VCDClient *govcd.VCDClient
	VDC       *govcd.Vdc // TODO: Incrementally remove and test in tests
	APIClient *swaggerClient.APIClient
	RWLock sync.RWMutex
}

//  TODO: Make sure this function still works properly with no issues after refactor
func (client *Client) RefreshBearerToken() error {
	klog.Infof("Refreshing vcd client")

	href := fmt.Sprintf("%s/api", client.VCDAuthConfig.Host)
	client.VCDClient.Client.APIVersion = VCloudApiVersion

	klog.Infof("Is user sysadmin: [%v]", client.VCDAuthConfig.IsSysAdmin)
	if client.VCDAuthConfig.RefreshToken != "" {
		userOrg := client.VCDAuthConfig.UserOrg
		if client.VCDAuthConfig.IsSysAdmin {
			userOrg = "system"
		}
		// Refresh vcd client using refresh token as system org user
		err := client.VCDClient.SetToken(userOrg,
			govcd.ApiTokenHeader, client.VCDAuthConfig.RefreshToken)
		if err != nil {
			return fmt.Errorf("failed to refresh VCD client with the refresh token: [%v]", err)
		}
	} else if client.VCDAuthConfig.User != "" && client.VCDAuthConfig.Password != "" {
		// Refresh vcd client using username and password
		resp, err := client.VCDClient.GetAuthResponse(client.VCDAuthConfig.User, client.VCDAuthConfig.Password,
			client.VCDAuthConfig.UserOrg)
		if err != nil {
			return fmt.Errorf("unable to authenticate [%s/%s] for url [%s]: [%+v] : [%v]",
				client.VCDAuthConfig.UserOrg, client.VCDAuthConfig.User, href, resp, err)
		}
	} else {
		return fmt.Errorf(
			"unable to find refresh token or secret to refresh vcd client for user [%s/%s] and url [%s]",
			client.VCDAuthConfig.UserOrg, client.VCDAuthConfig.User, href)
	}

	// reset legacy client
	org, err := client.VCDClient.GetOrgByNameOrId(client.ClusterOrgName)
	if err != nil {
		return fmt.Errorf("unable to get vcd organization [%s]: [%v]",
			client.ClusterOrgName, err)
	}

	vdc, err := org.GetVDCByName(client.ClusterOVDCName, true)
	if err != nil {
		return fmt.Errorf("unable to get VDC from org [%s], VDC [%s]: [%v]",
			client.ClusterOrgName, client.VCDAuthConfig.VDC, err)
	}
	client.VDC = vdc

	// reset swagger client
	swaggerConfig := swaggerClient.NewConfiguration()
	swaggerConfig.BasePath = fmt.Sprintf("%s/cloudapi", client.VCDAuthConfig.Host)
	swaggerConfig.AddDefaultHeader("Authorization", fmt.Sprintf("Bearer %s", client.VCDClient.Client.VCDToken))
	swaggerConfig.HTTPClient = &http.Client{
		Transport: &http.Transport{
			TLSClientConfig: &tls.Config{InsecureSkipVerify: client.VCDAuthConfig.Insecure},
		},
	}
	client.APIClient = swaggerClient.NewAPIClient(swaggerConfig)

	klog.Info("successfully refreshed all clients")
	return nil
}

// NewVCDClientFromSecrets :
// host, orgName, userOrg, refreshToken, insecure, user, password

// New method from (vdcClient, vdcName) return *govcd.Vdc
func NewVCDClientFromSecrets(host string, orgName string, vdcName string, userOrg string,
	user string, password string, refreshToken string, insecure bool, getVdcClient bool) (*Client, error) {

	// TODO: validation of parameters

	clientCreatorLock.Lock()
	defer clientCreatorLock.Unlock()

	// Return old client if everything matches. Else create new one and cache it.
	// This is suboptimal but is not a common case.
	if clientSingleton != nil {
		if clientSingleton.VCDAuthConfig.Host == host &&
			clientSingleton.ClusterOrgName == orgName &&
			clientSingleton.ClusterOVDCName == vdcName &&
			clientSingleton.VCDAuthConfig.UserOrg == userOrg &&
			clientSingleton.VCDAuthConfig.User == user &&
			clientSingleton.VCDAuthConfig.Password == password &&
			clientSingleton.VCDAuthConfig.RefreshToken == refreshToken &&
			clientSingleton.VCDAuthConfig.Insecure == insecure {
			return clientSingleton, nil
		}
	}

	vcdAuthConfig := NewVCDAuthConfigFromSecrets(host, user, password, refreshToken, userOrg, insecure) //

	vcdClient, apiClient, err := vcdAuthConfig.GetSwaggerClientFromSecrets()
	if err != nil {
		return nil, fmt.Errorf("unable to get swagger client from secrets: [%v]", err)
	}

	client := &Client{
		VCDAuthConfig:   vcdAuthConfig,
		ClusterOrgName:  orgName,
		ClusterOVDCName: vdcName,
		VCDClient:       vcdClient,
		APIClient:       apiClient,
	}

	if getVdcClient {
		org, err := vcdClient.GetOrgByName(orgName)
		if err != nil {
			return nil, fmt.Errorf("unable to get org from name [%s]: [%v]", orgName, err)
		}

		client.VDC, err = org.GetVDCByName(vdcName, true)
		if err != nil {
			return nil, fmt.Errorf("unable to get VDC [%s] from org [%s]: [%v]", vdcName, orgName, err)
		}
	}
	client.VCDClient = vcdClient
	clientSingleton = client

	klog.Infof("Client singleton is sysadmin: [%v]", clientSingleton.VCDClient.Client.IsSysAdmin)
	return clientSingleton, nil
}
