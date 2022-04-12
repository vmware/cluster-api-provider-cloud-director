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
	HTTP  int32 `yaml:"http"`
	HTTPS int32 `yaml:"https"`
	TCP   int32 `yaml:"tcp"`
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

// ClusterResourcesConfig :
type ClusterResourcesConfig struct {
	CsiVersion    string `yaml:"csi"`
	CpiVersion    string `yaml:"cpi"`
	CniVersion    string `yaml:"cni"`
	CapvcdVersion string
}

//CloudConfig contains the config that will be read from the secret
type CloudConfig struct {
	VCD                    VCDConfig              `yaml:"vcd"`
	ClusterID              string                 `yaml:"clusterid"`
	LB                     LBConfig               `yaml:"loadbalancer"`
	ManagementClusterRDEId string                 `yaml:"managementClusterRDEId,omitempty"`
	ClusterResources       ClusterResourcesConfig `yaml:"clusterResourceSet"`
	CapvcdVersion          string
}

type VCDDetails struct {
	VIPSubnet string `yaml:"vipSubnet"`
}

// CAPVCD config
type CAPVCDConfig struct {
	VCD                    VCDDetails             `yaml:"vcd"`
	LB                     LBConfig               `yaml:"loadbalancer"`
	ClusterResources       ClusterResourcesConfig `yaml:"clusterResourceSet"`
	ManagementClusterRDEId string                 `yaml:"managementClusterRDEId,omitempty"`
}

func ParseCAPVCDConfig(configReader io.Reader) (*CAPVCDConfig, error) {
	var err error
	config := &CAPVCDConfig{
		LB: LBConfig{
			OneArm: nil,
			Ports: Ports{
				HTTP:  80,
				HTTPS: 443,
				TCP:   6443,
			},
		},
		ClusterResources: ClusterResourcesConfig{
			CsiVersion: "1.1.1",
			CpiVersion: "1.1.1",
			CniVersion: "", // no default for antrea
		},
	}
	decoder := yaml.NewDecoder(configReader)
	decoder.SetStrict(true)

	if err = decoder.Decode(&config); err != nil {
		return nil, fmt.Errorf("Unable to decode yaml file: [%v]", err)
	}

	return config, nil
}

// ParseCloudConfig is used only in the tests
func ParseCloudConfig(configReader io.Reader) (*CloudConfig, error) {
	var err error
	config := &CloudConfig{
		LB: LBConfig{
			OneArm: nil,
			Ports: Ports{
				HTTP:  80,
				HTTPS: 443,
				TCP:   6443,
			},
		},
		ClusterResources: ClusterResourcesConfig{
			CsiVersion: "1.1.1",
			CpiVersion: "1.1.1",
			CniVersion: "", // no default for antrea
		},
	}
	decoder := yaml.NewDecoder(configReader)
	decoder.SetStrict(true)

	if err = decoder.Decode(&config); err != nil {
		return nil, fmt.Errorf("Unable to decode yaml file: [%v]", err)
	}

	return config, nil
}
