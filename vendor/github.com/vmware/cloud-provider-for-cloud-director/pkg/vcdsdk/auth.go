/*
   Copyright 2021 VMware, Inc.
   SPDX-License-Identifier: Apache-2.0
*/

package vcdsdk

import (
	"crypto/tls"
	"fmt"
	swaggerClient36 "github.com/vmware/cloud-provider-for-cloud-director/pkg/vcdswaggerclient_36_0"
	swaggerClient37 "github.com/vmware/cloud-provider-for-cloud-director/pkg/vcdswaggerclient_37_2"
	"github.com/vmware/go-vcloud-director/v2/govcd"
	"k8s.io/klog"
	"net/http"
	"net/url"
)

const (
	VCloudApiVersion_36_0 = "36.0"
	VCloudApiVersion_37_2 = "37.2"
)

// VCDAuthConfig : contains config related to vcd auth
type VCDAuthConfig struct {
	User         string `json:"user"`
	Password     string `json:"password"`
	RefreshToken string `json:"refreshToken"`
	UserOrg      string `json:"org"`
	Host         string `json:"host"`
	CloudAPIHref string `json:"cloudapihref"`
	VDC          string `json:"vdc"` // TODO: Get rid of
	Insecure     bool   `json:"insecure"`
	IsSysAdmin   bool   // will be set by GetBearerToken()
}

func (config *VCDAuthConfig) GetBearerToken() (*govcd.VCDClient, *http.Response, error) {
	href := fmt.Sprintf("%s/api", config.Host)
	u, err := url.ParseRequestURI(href)
	if err != nil {
		return nil, nil, fmt.Errorf("unable to parse url [%s]: %s", href, err)
	}

	vcdClient := govcd.NewVCDClient(*u, config.Insecure)
	// continue using API version 36.0 for GoVCD clients
	vcdClient.Client.APIVersion = VCloudApiVersion_36_0
	if err != nil {
		klog.Errorf("failed to set API version on GoVCD client: [%v]", err)
		return nil, nil, fmt.Errorf("failed to set API version on the GoVCD client: [%v]", err)
	}

	klog.Infof("Using VCD OpenAPI version [%s]", vcdClient.Client.APIVersion)

	var resp *http.Response
	if config.RefreshToken != "" {
		// NOTE: for a system admin user using refresh token, the userOrg will still be tenant org.
		// try setting authentication as a system org user
		err = vcdClient.SetToken("system",
			govcd.ApiTokenHeader, config.RefreshToken)
		if err != nil {
			klog.V(3).Infof("Authenticate as system org user didn't go through. Attempting as [%s] org user: [%v]", config.UserOrg, err)
			// failed to authenticate as system user. Retry as a tenant user
			err = vcdClient.SetToken(config.UserOrg,
				govcd.ApiTokenHeader, config.RefreshToken)
			if err != nil {
				klog.Errorf("failed to authenticate using refresh token")
				return nil, nil, fmt.Errorf("failed to set authorization header: [%v]", err)
			}
		} else {
			// No error while authenticating with "system" org.
			// The persisted userorg should be changed to "system"
			config.UserOrg = "system"
		}
		config.IsSysAdmin = vcdClient.Client.IsSysAdmin

		klog.Infof("Running module as sysadmin [%v]", vcdClient.Client.IsSysAdmin)
		return vcdClient, resp, nil
	}

	resp, err = vcdClient.GetAuthResponse(config.User, config.Password, config.UserOrg)
	if err != nil {
		return nil, resp, fmt.Errorf("unable to authenticate [%s/%s] for url [%s]: [%+v] : [%v]",
			config.UserOrg, config.User, href, resp, err)
	}

	return vcdClient, resp, nil
}

func (config *VCDAuthConfig) GetSwaggerClientFromSecrets() (*govcd.VCDClient, *swaggerClient36.APIClient, *swaggerClient37.APIClient, error) {

	vcdClient, _, err := config.GetBearerToken()
	if err != nil {
		return nil, nil, nil, fmt.Errorf("unable to get bearer token from secrets: [%v]", err)
	}
	authHeader := fmt.Sprintf("Bearer %s", vcdClient.Client.VCDToken)

	swaggerConfig := swaggerClient36.NewConfiguration()
	swaggerConfig.BasePath = fmt.Sprintf("%s/cloudapi", config.Host)
	swaggerConfig.AddDefaultHeader("Authorization", authHeader)
	swaggerConfig.HTTPClient = &http.Client{
		Transport: &http.Transport{
			TLSClientConfig: &tls.Config{InsecureSkipVerify: config.Insecure},
		},
	}
	apiClient36 := swaggerClient36.NewAPIClient(swaggerConfig)

	// initialize swagger client for API version 37.2 only if the API version 37.2 is available
	var apiClient37 *swaggerClient37.APIClient
	if vcdClient.Client.APIVCDMaxVersionIs(fmt.Sprintf(">=%s", VCloudApiVersion_37_2)) {
		swaggerConfig37 := swaggerClient37.NewConfiguration()
		swaggerConfig37.BasePath = fmt.Sprintf("%s/cloudapi", config.Host)
		swaggerConfig37.AddDefaultHeader("Authorization", authHeader)
		swaggerConfig37.HTTPClient = &http.Client{
			Transport: &http.Transport{
				TLSClientConfig: &tls.Config{InsecureSkipVerify: config.Insecure},
			},
		}
		apiClient37 = swaggerClient37.NewAPIClient(swaggerConfig37)
	}
	return vcdClient, apiClient36, apiClient37, nil
}

func (config *VCDAuthConfig) GetPlainClientFromSecrets() (*govcd.VCDClient, error) {

	href := fmt.Sprintf("%s/api", config.Host)
	u, err := url.ParseRequestURI(href)
	if err != nil {
		return nil, fmt.Errorf("unable to parse url: [%s]: [%v]", href, err)
	}

	vcdClient := govcd.NewVCDClient(*u, config.Insecure)
	// continue using API version 36 for GoVCD clients
	vcdClient.Client.APIVersion = VCloudApiVersion_36_0
	klog.Infof("Using VCD XML API version [%s]", vcdClient.Client.APIVersion)
	if err = vcdClient.Authenticate(config.User, config.Password, config.UserOrg); err != nil {
		return nil, fmt.Errorf("cannot authenticate with vcd: [%v]", err)
	}

	return vcdClient, nil
}

func NewVCDAuthConfigFromSecrets(host string, user string, secret string,
	refreshToken string, userOrg string, insecure bool) *VCDAuthConfig {
	return &VCDAuthConfig{
		Host:         host,
		User:         user,
		Password:     secret,
		RefreshToken: refreshToken,
		UserOrg:      userOrg,
		Insecure:     insecure,
	}
}

func SetClientAPIVersion(cli *govcd.VCDClient) error {
	if cli == nil {
		return fmt.Errorf("GoVCD client is nil")
	}
	if cli.Client.APIVCDMaxVersionIs(fmt.Sprintf(">=%s", VCloudApiVersion_37_2)) {
		cli.Client.APIVersion = VCloudApiVersion_37_2
		return nil
	}
	cli.Client.APIVersion = VCloudApiVersion_36_0
	return nil
}
