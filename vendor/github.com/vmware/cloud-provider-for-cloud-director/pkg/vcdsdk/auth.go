/*
   Copyright 2021 VMware, Inc.
   SPDX-License-Identifier: Apache-2.0
*/

package vcdsdk

import (
	"crypto/tls"
	"fmt"
	swaggerClient "github.com/vmware/cloud-provider-for-cloud-director/pkg/vcdswaggerclient"
	"github.com/vmware/go-vcloud-director/v2/govcd"
	"k8s.io/klog"
	"net/http"
	"net/url"
)

const (
	VCloudApiVersion = "36.0"
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
	vcdClient.Client.APIVersion = VCloudApiVersion
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

func (config *VCDAuthConfig) GetSwaggerClientFromSecrets() (*govcd.VCDClient, *swaggerClient.APIClient, error) {

	vcdClient, _, err := config.GetBearerToken()
	if err != nil {
		return nil, nil, fmt.Errorf("unable to get bearer token from secrets: [%v]", err)
	}
	authHeader := fmt.Sprintf("Bearer %s", vcdClient.Client.VCDToken)

	swaggerConfig := swaggerClient.NewConfiguration()
	swaggerConfig.BasePath = fmt.Sprintf("%s/cloudapi", config.Host)
	swaggerConfig.AddDefaultHeader("Authorization", authHeader)
	swaggerConfig.HTTPClient = &http.Client{
		Transport: &http.Transport{
			TLSClientConfig: &tls.Config{InsecureSkipVerify: config.Insecure},
		},
	}

	return vcdClient, swaggerClient.NewAPIClient(swaggerConfig), nil
}

func (config *VCDAuthConfig) GetPlainClientFromSecrets() (*govcd.VCDClient, error) {

	href := fmt.Sprintf("%s/api", config.Host)
	u, err := url.ParseRequestURI(href)
	if err != nil {
		return nil, fmt.Errorf("unable to parse url: [%s]: [%v]", href, err)
	}

	vcdClient := govcd.NewVCDClient(*u, config.Insecure)
	vcdClient.Client.APIVersion = VCloudApiVersion
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
