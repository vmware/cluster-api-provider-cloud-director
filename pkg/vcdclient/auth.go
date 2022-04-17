/*
   Copyright 2021 VMware, Inc.
   SPDX-License-Identifier: Apache-2.0
*/

package vcdclient

import (
	"crypto/tls"
	"fmt"
	"net/http"
	"net/url"
	"runtime"
	"strings"

	swaggerClient "github.com/vmware/cluster-api-provider-cloud-director/pkg/vcdswaggerclient"
	"github.com/vmware/cluster-api-provider-cloud-director/release"
	"github.com/vmware/go-vcloud-director/v2/govcd"
	"k8s.io/klog"
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
	VDC          string `json:"Vdc"`
	Insecure     bool   `json:"insecure"`
	Token        string `json:"token"`
	IsSysAdmin   bool   // will be set by GetBearerToken()
}

func buildUserAgent() string {
	return fmt.Sprintf("cluster-api-vcloud-director/%s (%s/%s; isProvider:false)", strings.TrimSuffix(release.CapVCDVersion, "\n"), runtime.GOOS, runtime.GOARCH)
}

func (config *VCDAuthConfig) GetBearerToken() (*govcd.VCDClient, *http.Response, error) {
	klog.V(10).Infof("VCDAuthConfig: %+v\n", config)

	href := fmt.Sprintf("%s/api", config.Host)
	u, err := url.ParseRequestURI(href)
	if err != nil {
		return nil, nil, fmt.Errorf("unable to parse url [%s]: %s", href, err)
	}

	vcdClient := govcd.NewVCDClient(*u, config.Insecure, govcd.WithHttpUserAgent(buildUserAgent()))
	vcdClient.Client.APIVersion = VCloudApiVersion
	klog.Infof("Using VCD OpenAPI version [%s]", vcdClient.Client.APIVersion)

	var resp *http.Response
	if config.RefreshToken != "" {
		err = vcdClient.SetToken(config.UserOrg,
			govcd.ApiTokenHeader, config.RefreshToken)
		if err != nil {
			return nil, nil, fmt.Errorf("failed to set authorization header: [%v]", err)
		}
		config.IsSysAdmin = vcdClient.Client.IsSysAdmin
		if err != nil {
			return nil, nil, fmt.Errorf("failed to determine if the user is a system administrator: [%v]", err)
		}
		vcdClient.Client.IsSysAdmin = config.IsSysAdmin

		klog.Infof("Running CAPVCD as sysadmin? [%v]", vcdClient.Client.IsSysAdmin)
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

	swaggerConfig := swaggerClient.NewConfiguration()
	authHeader := fmt.Sprintf("Bearer %s", vcdClient.Client.VCDToken)
	swaggerConfig.BasePath = fmt.Sprintf("%s/cloudapi", config.Host)
	swaggerConfig.AddDefaultHeader("Authorization", authHeader)
	swaggerConfig.UserAgent = buildUserAgent()
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

	vcdClient := govcd.NewVCDClient(*u, config.Insecure, govcd.WithHttpUserAgent(buildUserAgent()))
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
