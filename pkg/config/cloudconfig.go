/*
   Copyright 2021 VMware, Inc.
   SPDX-License-Identifier: Apache-2.0
*/

/*
   Copyright 2021 VMware, Inc.
   SPDX-License-Identifier: Apache-2.0
*/

package config

import (
	"fmt"
	yaml "gopkg.in/yaml.v2"
	"io"
	"io/ioutil"
	"k8s.io/klog"
	"strings"
)

// VCDConfig :
type VCDConfig struct {
	Host    string `yaml:"host"`
	VDC     string `yaml:"vdc"`
	Org     string `yaml:"org"`
	UserOrg string // this defaults to Org or a prefix of User

	// It is allowed to pass the following variables using the config. However
	// that is unsafe security practice. However there can be user scenarios and
	// testing scenarios where this is sensible.
	User         string
	Secret       string
	RefreshToken string

	VDCNetwork string `yaml:"network"`
	VIPSubnet  string `yaml:"vipSubnet"`
}

// Ports :
type Ports struct {
	HTTP  int32 `yaml:"http" default:"80"`
	HTTPS int32 `yaml:"https" default:"443"`
	TCP   int32 `yaml:"tcp" default:"6443"`
}

// OneArm :
type OneArm struct {
	StartIP string `yaml:"startIP"`
	EndIP   string `yaml:"endIP"`
}

// LBConfig :
type LBConfig struct {
	OneArm *OneArm `yaml:"oneArm,omitempty"`
	Ports  Ports   `yaml:"ports"`
}

// CloudConfig contains the config that will be read from the secret
type CloudConfig struct {
	VCD                    VCDConfig `yaml:"vcd"`
	ClusterID              string    `yaml:"clusterid"`
	LB                     LBConfig  `yaml:"loadbalancer"`
	ManagementClusterRDEId string    `yaml:"managementClusterRDEId,omitempty"`
}

func getUserAndOrg(fullUserName string, clusterOrg string) (userOrg string, userName string, err error) {
	// If the full username is specified as org/user, the scenario is that the user
	// may belong to an org different from the cluster, but still has the
	// necessary rights to view the VMs on this org. Else if the username is
	// specified as just user, the scenario is that the user is in the same org
	// as the cluster.
	parts := strings.Split(string(fullUserName), "/")
	if len(parts) > 2 {
		return "", "", fmt.Errorf(
			"invalid username format; expected at most two fields separated by /, obtained [%d]",
			len(parts))
	}
	if len(parts) == 1 {
		userOrg = clusterOrg
		userName = parts[0]
	} else {
		userOrg = parts[0]
		userName = parts[1]
	}

	return userOrg, userName, nil
}

// ParseCloudConfig : parses config and env to fill in the CloudConfig struct
func ParseCloudConfig(configReader io.Reader) (*CloudConfig, error) {
	var err error
	config := &CloudConfig{}

	decoder := yaml.NewDecoder(configReader)
	decoder.SetStrict(true)

	if err = decoder.Decode(&config); err != nil {
		return nil, fmt.Errorf("Unable to decode yaml file: [%v]", err)
	}

	return config, nil
}

func SetAuthorization(config *CloudConfig) error {
	refreshToken, err := ioutil.ReadFile("/etc/kubernetes/vcloud/basic-auth/refreshToken")
	if err != nil {
		klog.Infof("Unable to get refresh token: [%v]", err)
	} else {
		config.VCD.RefreshToken = strings.TrimSuffix(string(refreshToken), "\n")
	}

	username, err := ioutil.ReadFile("/etc/kubernetes/vcloud/basic-auth/username")
	if err != nil {
		klog.Infof("Unable to get username: [%v]", err)
	} else {
		if string(username) != "" {
			config.VCD.UserOrg, config.VCD.User, err = getUserAndOrg(string(username), config.VCD.Org)
			if err != nil {
				return fmt.Errorf("unable to get user org and name: [%v]", err)
			}
		} else {
			config.VCD.UserOrg = strings.TrimSuffix(config.VCD.Org, "\n")
		}
	}

	secret, err := ioutil.ReadFile("/etc/kubernetes/vcloud/basic-auth/password")
	if err != nil {
		klog.Infof("Unable to get password: [%v]", err)
	} else {
		config.VCD.Secret = strings.TrimSuffix(string(secret), "\n")
	}

	if config.VCD.RefreshToken != "" {
		klog.Infof("Using non-empty refresh token.")
		return nil
	}
	if config.VCD.User != "" && config.VCD.UserOrg != "" && config.VCD.Secret != "" {
		klog.Infof("Using username/secret based credentials.")
		return nil
	}

	return fmt.Errorf("unable to get valid set of credentials from secrets")
}

func ValidateCloudConfig(config *CloudConfig) error {
	// TODO: needs more validation
	if config == nil {
		return fmt.Errorf("nil config passed")
	}

	if config.VCD.Host == "" {
		return fmt.Errorf("need a valid vCloud Host")
	}
	if config.VCD.VDCNetwork == "" {
		return fmt.Errorf("need a valid ovdc network name")
	}

	return nil
}
