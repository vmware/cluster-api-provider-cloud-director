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
	"gopkg.in/yaml.v2"
	"io/ioutil"
	"os"
	"path/filepath"
	"strings"
)

var (
	gitRoot string = ""
)

type VcdInfo struct {
	Host         string `yaml:"host"`
	TenantOrg    string `yaml:"tenantOrg"`
	TenantVdc    string `yaml:"tenantVdc"`
	OvdcNetwork  string `yaml:"ovdcNetwork"`
	User         string `yaml:"user"`
	UserOrg      string `yaml:"userOrg"`
	Password     string `yaml:"password"`
	RefreshToken string `yaml:"refreshToken"`
}

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

	// "testdata/vcd_info.yaml" contains VCD details
	testVcdInfoFilePath := filepath.Join(gitRoot, "testdata/vcd_info.yaml")
	vcdInfoContent, err := ioutil.ReadFile(testVcdInfoFilePath)
	if err != nil {
		return nil, fmt.Errorf("error reading the vcd_info.yaml file contents: [%v]", err)
	}

	var vcdInfo VcdInfo
	err = yaml.Unmarshal(vcdInfoContent, &vcdInfo)
	if err != nil {
		return nil, fmt.Errorf("error parsing vcd_info.yaml file content: [%v]", err)
	}

	// "testdata/config_test.yaml" contains all the information that can be passed in as the config map
	testConfigFilePath := filepath.Join(gitRoot, "testdata/config_test.yaml")
	capvcdVersionFile := filepath.Join(gitRoot, "release/version")
	capvcdVersion, err := ioutil.ReadFile(capvcdVersionFile)
	if err != nil {
		return nil, fmt.Errorf("failed to get CAPVCD version from file [%s]: [%v]", capvcdVersionFile, err)
	}
	configReader, err := os.Open(testConfigFilePath)
	if err != nil {
		return nil, fmt.Errorf("unable to open file [%s]: [%v]", testConfigFilePath, err)
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
				vcdInfo.Host = getStrValStrict(val, vcdInfo.Host)
				vcdInfo.Host = strings.TrimRight(vcdInfo.Host, "/")
			case "org":
				vcdInfo.TenantOrg = getStrValStrict(val, vcdInfo.TenantOrg)
			case "network":
				vcdInfo.OvdcNetwork = getStrValStrict(val, vcdInfo.OvdcNetwork)
			case "subnet":
				cloudConfig.VCD.VIPSubnet = getStrValStrict(val, cloudConfig.VCD.VIPSubnet)
			case "user":
				vcdInfo.User = getStrValStrict(val, vcdInfo.User)
			case "secret":
				vcdInfo.Password = getStrValStrict(val, vcdInfo.Password)
			case "userOrg":
				// default to userOrg if val is empty
				vcdInfo.UserOrg = getStrValStrict(val, vcdInfo.UserOrg)
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
			case "refreshToken":
				vcdInfo.RefreshToken = getStrValStrict(val, cloudConfig.VCD.RefreshToken)
			}
		}
	}

	return NewVCDClientFromSecrets(
		vcdInfo.Host,
		vcdInfo.TenantOrg,
		vcdInfo.TenantVdc,
		"",
		vcdInfo.OvdcNetwork,
		cloudConfig.VCD.VIPSubnet,
		vcdInfo.UserOrg,
		vcdInfo.User,
		vcdInfo.Password,
		vcdInfo.RefreshToken,
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
		string(capvcdVersion),
	)
}
