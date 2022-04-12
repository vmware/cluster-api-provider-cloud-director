/*
   Copyright 2021 VMware, Inc.
   SPDX-License-Identifier: Apache-2.0
*/

/*
   Copyright 2021 VMware, Inc.
   SPDX-License-Identifier: Apache-2.0
*/

package vcdclient

import (
	"gopkg.in/yaml.v2"
	"io/ioutil"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
)

type authorizationDetails struct {
	Username               string `yaml:"username"`
	Password               string `yaml:"password"`
	RefreshToken           string `yaml:"refreshToken"`
	UserOrg                string `yaml:"userOrg"`
	SystemUser             string `yaml:"systemUser"`
	SystemUserPassword     string `yaml:"systemUserPassword"`
	SystemUserRefreshToken string `yaml:"systemUserRefreshToken"`
}

func TestNewVCDAuthConfigFromSecrets(t *testing.T) {

	authFile := filepath.Join(gitRoot, "testdata/auth_test.yaml")
	authFileContent, err := ioutil.ReadFile(authFile)
	assert.NoError(t, err, "There should be no error reading the auth file contents.")

	var authDetails authorizationDetails
	err = yaml.Unmarshal(authFileContent, &authDetails)
	assert.NoError(t, err, "There should be no error parsing auth file content.")

	vcdClient, err := getTestVCDClient(map[string]interface{}{
		"user":    authDetails.Username,
		"secret":  authDetails.Password,
		"userOrg": authDetails.UserOrg,
	})
	assert.NoError(t, err, "Unable to get VCD client")
	assert.NotNil(t, vcdClient, "VCD Client should not be nil")

	vcdClient, err = getTestVCDClient(map[string]interface{}{
		"getVdcClient": true,
		"user":         authDetails.Username,
		"secret":       authDetails.Password,
		"userOrg":      authDetails.UserOrg,
	})
	assert.NoError(t, err, "Unable to get Client with VDC details.")

	vcdClient, err = getTestVCDClient(map[string]interface{}{
		"refreshToken": authDetails.RefreshToken,
		"userOrg":      authDetails.UserOrg,
	})
	assert.NoError(t, err, "Unable to get VCD Client")
	assert.NotNil(t, vcdClient, "VCD Client should not be nil")

	vcdClient, err = getTestVCDClient(map[string]interface{}{
		"host":    "https://some-random-address",
		"user":    authDetails.Username,
		"secret":  authDetails.Password,
		"userOrg": authDetails.UserOrg,
	})
	assert.Error(t, err, "Error should be obtained for url [https://some-random-address]")

	vcdClient, err = getTestVCDClient(map[string]interface{}{
		"network": "",
		"user":    authDetails.Username,
		"secret":  authDetails.Password,
		"userOrg": authDetails.UserOrg,
	})
	assert.Error(t, err, "Error should be obtained for missing network")

	vcdClient, err = getTestVCDClient(map[string]interface{}{
		"oneArm":  nil,
		"user":    authDetails.Username,
		"secret":  authDetails.Password,
		"userOrg": authDetails.UserOrg,
	})
	assert.NoError(t, err, "There should be no error for missing OneArm")

	vcdClient, err = getTestVCDClient(map[string]interface{}{
		"user":    authDetails.SystemUser,
		"secret":  authDetails.SystemUserPassword,
		"userOrg": "system",
	})
	assert.NoError(t, err, "Unable to get VCD client for system administrator")
	assert.NotNil(t, vcdClient, "VCD Client should not be nil")

	vcdClient, err = getTestVCDClient(map[string]interface{}{
		"user":    authDetails.SystemUser,
		"secret":  authDetails.SystemUserPassword,
		"userOrg": "system",
		"network": "",
	})
	assert.Error(t, err, "Error should be obtained for missing network")

	vcdClient, err = getTestVCDClient(map[string]interface{}{
		"refreshToken": authDetails.SystemUserRefreshToken,
		"userOrg":      "system",
	})
	assert.NoError(t, err, "Unable to get VCD client for system administrator")
	assert.NotNil(t, vcdClient, "VCD Client should not be nil")

	return
}
