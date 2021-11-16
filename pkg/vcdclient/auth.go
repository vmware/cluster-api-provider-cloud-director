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

	swaggerClient "github.com/vmware/cluster-api-provider-cloud-director/pkg/vcdswaggerclient"
	"github.com/vmware/go-vcloud-director/v2/govcd"
	"k8s.io/klog"
)

// VCDAuthConfig : contains config related to vcd auth
type VCDAuthConfig struct {
	User         string `json:"user"`
	Password     string `json:"password"`
	Org          string `json:"org"`
	Host         string `json:"host"`
	CloudAPIHref string `json:"cloudapihref"`
	VDC          string `json:"Vdc"`
	Insecure     bool   `json:"insecure"`
	Token        string `json:"token"`
}

func (config *VCDAuthConfig) GetBearerTokenFromSecrets() (*govcd.VCDClient, *http.Response, error) {
	href := fmt.Sprintf("%s/api", config.Host)
	u, err := url.ParseRequestURI(href)
	if err != nil {
		return nil, nil, fmt.Errorf("unable to parse url [%s]: %s", href, err)
	}

	vcdClient := govcd.NewVCDClient(*u, config.Insecure)
	vcdClient.Client.APIVersion = "35.0"
	klog.Infof("Using VCD OpenAPI version [%s]", vcdClient.Client.APIVersion)

	resp, err := vcdClient.GetAuthResponse(config.User, config.Password, config.Org)
	if err != nil {
		return nil, resp, fmt.Errorf("unable to authenticate [%s/%s] for url [%s]: [%+v] : [%v]",
			config.Org, config.User, href, resp, err)
	}

	return vcdClient, resp, nil
}

func (config *VCDAuthConfig) GetSwaggerClientFromSecrets() (*govcd.VCDClient, *swaggerClient.APIClient, error) {

	vcdClient, _, err := config.GetBearerTokenFromSecrets()
	if err != nil {
		return nil, nil, fmt.Errorf("unable to get bearer token from serets: [%v]", err)
	}
	authHeader := fmt.Sprintf("Bearer %s", vcdClient.Client.VCDToken)

	swaggerConfig := swaggerClient.NewConfiguration()
	swaggerConfig.BasePath = fmt.Sprintf("%s/cloudapi", config.Host)
	swaggerConfig.AddDefaultHeader("Authorization", authHeader)
	swaggerConfig.HTTPClient = &http.Client{
		Transport: &http.Transport{
			TLSClientConfig: &tls.Config{InsecureSkipVerify: true},
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
	vcdClient.Client.APIVersion = "35.0"
	klog.Infof("Using VCD XML API version [%s]", vcdClient.Client.APIVersion)
	if err = vcdClient.Authenticate(config.User, config.Password, config.Org); err != nil {
		return nil, fmt.Errorf("cannot authenticate with vcd: [%v]", err)
	}

	return vcdClient, nil
}

func NewVCDAuthConfigFromSecrets(host string, user string, secret string, org string, insecure bool, ovdc string) *VCDAuthConfig {
	return &VCDAuthConfig{
		Host:     host,
		User:     user,
		Password: secret,
		Org:      org,
		Insecure: insecure,
		VDC:      ovdc,
	}
}
