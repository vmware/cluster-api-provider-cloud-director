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
	"fmt"
	"github.com/vmware/cluster-api-provider-cloud-director/pkg/config"
	"os"
	"path/filepath"
)

var (
	gitRoot string = ""
)

func init() {
	gitRoot = os.Getenv("GITROOT")
	if gitRoot == "" {
		// It is okay to panic here as this will be caught during dev
		panic("GITROOT should be set")
	}
}

func getStrValStrict(val interface{}, defaultVal string) string {
	if strVal, ok := val.(string); ok {
		return strVal
	}

	return defaultVal
}

func getBoolValStrict(val interface{}, defaultVal bool) bool {
	if boolVal, ok := val.(bool); ok {
		return boolVal
	}

	return defaultVal
}

func getOneArmValStrict(val interface{}, defaultVal *OneArm) *OneArm {
	if oneArmVal, ok := val.(*OneArm); ok {
		return oneArmVal
	}

	return defaultVal
}

func getInt32ValStrict(val interface{}, defaultVal int32) int32 {
	if int32Val, ok := val.(int32); ok {
		return int32Val
	}

	return defaultVal
}

func getTestVCDClient(inputMap map[string]interface{}) (*Client, error) {

	testConfigFilePath := filepath.Join(gitRoot, "testdata/config_test.yaml")
	configReader, err := os.Open(testConfigFilePath)
	if err != nil {
		return nil, fmt.Errorf("Unable to open file [%s]: [%v]", testConfigFilePath, err)
	}
	defer configReader.Close()

	cloudConfig, err := config.ParseCloudConfig(configReader)
	if err != nil {
		return nil, fmt.Errorf("Unable to parse cloud config file [%s]: [%v]", testConfigFilePath, err)
	}

	insecure := true
	oneArm := &OneArm{
		StartIPAddress: cloudConfig.LB.OneArm.StartIP,
		EndIPAddress:   cloudConfig.LB.OneArm.EndIP,
	}
	getVdcClient := false
	if inputMap != nil {
		for key, val := range inputMap {
			switch key {
			case "host":
				cloudConfig.VCD.Host = getStrValStrict(val, cloudConfig.VCD.Host)
			case "org":
				cloudConfig.VCD.Org = getStrValStrict(val, cloudConfig.VCD.Org)
			case "network":
				cloudConfig.VCD.VDCNetwork = getStrValStrict(val, cloudConfig.VCD.VDCNetwork)
			case "subnet":
				cloudConfig.VCD.VIPSubnet = getStrValStrict(val, cloudConfig.VCD.VIPSubnet)
			case "user":
				cloudConfig.VCD.User = getStrValStrict(val, cloudConfig.VCD.User)
			case "secret":
				cloudConfig.VCD.Secret = getStrValStrict(val, cloudConfig.VCD.Secret)
			case "insecure":
				insecure = getBoolValStrict(val, true)
			case "clusterID":
				cloudConfig.ClusterID = getStrValStrict(val, cloudConfig.ClusterID)
			case "oneArm":
				oneArm = getOneArmValStrict(val, oneArm)
			case "httpPort":
				cloudConfig.LB.Ports.HTTP = getInt32ValStrict(val, cloudConfig.LB.Ports.HTTP)
			case "httpsPort":
				cloudConfig.LB.Ports.HTTPS = getInt32ValStrict(val, cloudConfig.LB.Ports.HTTPS)
			case "tcp":
				cloudConfig.LB.Ports.TCP = getInt32ValStrict(val, cloudConfig.LB.Ports.TCP)
			case "getVdcClient":
				getVdcClient = getBoolValStrict(val, false)
			}
		}
	}

	return NewVCDClientFromSecrets(
		cloudConfig.VCD.Host,
		cloudConfig.VCD.Org,
		cloudConfig.VCD.VDC,
		"",
		cloudConfig.VCD.VDCNetwork,
		cloudConfig.VCD.VIPSubnet,
		cloudConfig.VCD.UserOrg,
		cloudConfig.VCD.User,
		cloudConfig.VCD.Secret,
		cloudConfig.VCD.RefreshToken,
		insecure,
		cloudConfig.ClusterID,
		oneArm,
		cloudConfig.LB.Ports.HTTP,
		cloudConfig.LB.Ports.HTTPS,
		cloudConfig.LB.Ports.TCP,
		getVdcClient,
		"",
		cloudConfig.ClusterResources.CsiVersion,
		cloudConfig.ClusterResources.CpiVersion,
		cloudConfig.ClusterResources.CniVersion,
	)
}
