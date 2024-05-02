/*
   Copyright 2021 VMware, Inc.
   SPDX-License-Identifier: Apache-2.0
*/

package vcdsdk

import (
	"crypto/tls"
	"fmt"
	swaggerClient37 "github.com/vmware/cloud-provider-for-cloud-director/pkg/vcdswaggerclient_37_2"
	"k8s.io/klog"
	"net/http"
	"strings"
	"sync"

	"github.com/vmware/go-vcloud-director/v2/govcd"
)

// Client :
type Client struct {
	VCDAuthConfig         *VCDAuthConfig
	ClusterOrgName        string
	ClusterOVDCIdentifier string
	VCDClient             *govcd.VCDClient
	VDC                   *govcd.Vdc // TODO: Incrementally remove and test in tests
	APIClient             *swaggerClient37.APIClient
	RWLock                sync.RWMutex
}

func GetUserAndOrg(fullUserName string, clusterOrg string, currentUserOrg string) (userOrg string, userName string, err error) {
	// If the full username is specified as org/user, the scenario is that the user
	// may belong to an org different from the cluster, but still has the
	// necessary rights to view the VMs on this org. Else if the username is
	// specified as just user, the scenario is that the user is in the same org
	// as the cluster.
	parts := strings.Split(fullUserName, "/")
	if len(parts) > 2 {
		return "", "", fmt.Errorf(
			"invalid username format; expected at most two fields separated by /, obtained [%d]",
			len(parts))
	}
	// Add additional fallback to clusterOrg if current userOrg does not exist, this allows auth to continue properly
	if len(parts) == 1 {
		if currentUserOrg == "" {
			userOrg = clusterOrg
		} else {
			userOrg = currentUserOrg
		}
		userName = parts[0]
	} else {
		userOrg = parts[0]
		userName = parts[1]
	}

	return userOrg, userName, nil
}

// RefreshBearerToken gets a new Bearer Token from an API token
func (client *Client) RefreshBearerToken() error {
	klog.Infof("Refreshing vcd client")

	href := fmt.Sprintf("%s/api", client.VCDAuthConfig.Host)
	// continue using API version 36 for GoVCD client
	client.VCDClient.Client.APIVersion = VCloudApiVersion_37_2

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
	// Update client VDC if cluster org is provided
	if client.ClusterOrgName != "" {
		org, err := client.VCDClient.GetOrgByNameOrId(client.ClusterOrgName)
		if err != nil {
			return fmt.Errorf("unable to get vcd organization [%s]: [%v]",
				client.ClusterOrgName, err)
		}
		vdc, err := org.GetVDCByNameOrId(client.ClusterOVDCIdentifier, true)
		if err != nil {
			return fmt.Errorf("unable to get VDC from org [%s], VDC [%s]: [%v]",
				client.ClusterOrgName, client.VCDAuthConfig.VDC, err)
		}
		client.VDC = vdc
	}

	// reset swagger client
	swaggerConfig := swaggerClient37.NewConfiguration()
	swaggerConfig.BasePath = fmt.Sprintf("%s/cloudapi", client.VCDAuthConfig.Host)
	swaggerConfig.AddDefaultHeader("Authorization", fmt.Sprintf("Bearer %s", client.VCDClient.Client.VCDToken))
	swaggerConfig.HTTPClient = &http.Client{
		Transport: &http.Transport{
			TLSClientConfig: &tls.Config{InsecureSkipVerify: client.VCDAuthConfig.Insecure},
		},
	}
	client.APIClient = swaggerClient37.NewAPIClient(swaggerConfig)

	// initialize swagger client for API version 37.2 only if API version 37.2 is available
	if client.VCDClient.Client.APIVCDMaxVersionIs(fmt.Sprintf(">=%s", VCloudApiVersion_37_2)) {
		swaggerConfig37 := swaggerClient37.NewConfiguration()
		swaggerConfig37.BasePath = fmt.Sprintf("%s/cloudapi", client.VCDAuthConfig.Host)
		swaggerConfig37.AddDefaultHeader("Authorization", fmt.Sprintf("Bearer %s", client.VCDClient.Client.VCDToken))
		swaggerConfig37.HTTPClient = &http.Client{
			Transport: &http.Transport{
				TLSClientConfig: &tls.Config{InsecureSkipVerify: client.VCDAuthConfig.Insecure},
			},
		}
		client.APIClient = swaggerClient37.NewAPIClient(swaggerConfig37)
	}

	klog.Info("successfully refreshed all clients")
	return nil
}

// NewVCDClientFromSecrets :
// host, orgName, userOrg, refreshToken, insecure, user, password

// New method from (vdcClient, vdcIdentifier) return *govcd.Vdc
func NewVCDClientFromSecrets(host string, orgName string, vdcIdentifier string, userOrg string,
	user string, password string, refreshToken string, insecure bool, getVdcClient bool) (*Client, error) {

	// TODO: validation of parameters

	// When getting the client from main.go, the user, orgName, userOrg would have correct values due to config.SetAuthorization()
	// when user is sys/admin, userOrg and orgName will have different values, hence we need an additional parameter check to prevent overwrite
	// as now user='admin' and userOrg='system', we would enter the fallback to clusterOrg which would return userOrg=clusterOrg
	// so if userOrg is already set, we want the updated fallback to userOrg first which could fall back to clusterOrg if empty
	// In vcdcluster controller's case, both orgName and userOrg will be the same as we pass in vcdcluster.Spec.Org to both
	// but since username is still 'sys/admin', we will return correctly

	// TODO: Remove pkg/config dependency from vcdsdk; currently common_system_test.go depends on pkg/config
	newUserOrg, newUsername, err := GetUserAndOrg(user, orgName, userOrg)
	if err != nil {
		return nil, fmt.Errorf("error parsing username before authenticating to VCD: [%v]", err)
	}

	vcdAuthConfig := NewVCDAuthConfigFromSecrets(host, newUsername, password, refreshToken, newUserOrg, insecure) //

	vcdClient, apiClient, err := vcdAuthConfig.GetSwaggerClientFromSecrets()
	if err != nil {
		return nil, fmt.Errorf("unable to get swagger client from secrets: [%v]", err)
	}

	// We want to verify that user/pass is correct by getting the auth response. Unfortunately, govcd does not provide
	// the appropriate errors correlating to the http status codes, so we have to do this in this manner. However, if
	// the refresh token is set then we don't want to do this. There is no analogous method for testing auth response
	// with the token right now, so we'll have to do without for the time being.
	if refreshToken == "" {
		resp, err := vcdClient.GetAuthResponse(newUsername, password, newUserOrg)
		if err != nil {
			return nil, fmt.Errorf("error getting auth response from VCD with username [%s] and org [%s]: [%v]", newUsername, newUserOrg, err)
		}
		if resp.StatusCode != http.StatusOK {
			return nil, fmt.Errorf("failed to authenticate with VCD with username [%s] and org [%s]: [%s]", newUsername, newUserOrg, resp.Status)
		}
	}

	client := &Client{
		VCDAuthConfig:         vcdAuthConfig,
		ClusterOrgName:        orgName,
		ClusterOVDCIdentifier: vdcIdentifier,
		VCDClient:             vcdClient,
		APIClient:             apiClient,
	}

	if getVdcClient {
		org, err := vcdClient.GetOrgByName(orgName)
		if err != nil {
			return nil, fmt.Errorf("unable to get org from name [%s]: [%v]", orgName, err)
		}

		client.VDC, err = org.GetVDCByNameOrId(vdcIdentifier, true)
		if err != nil {
			return nil, fmt.Errorf("unable to get VDC [%s] from org [%s]: [%v]", vdcIdentifier, orgName, err)
		}
	}
	client.VCDClient = vcdClient

	klog.Infof("Client is sysadmin: [%v]", client.VCDClient.Client.IsSysAdmin)
	return client, nil
}
