package utils

import (
	"fmt"
	"github.com/vmware/cloud-provider-for-cloud-director/pkg/testingsdk"
)

const (
	timeoutMinutes           = 40
	pollIntervalSeconds      = 120
	machinePhaseProvisioned  = "Provisioned"
	machinePhaseProvisioning = "Provisioning"
	machinePhaseRunning      = "Running"
	VCDCluster               = "VCDCluster"
	Cluster                  = "Cluster"
	SECRET                   = "Secret"
	KubeadmControlPlane      = "KubeadmControlPlane"
	VCDMachineTemplate       = "VCDMachineTemplate"
	KubeadmConfigTemplate    = "KubeadmConfigTemplate"
	MachineDeployment        = "MachineDeployment"
)

type patchStringValue struct {
	Op    string `json:"op"`
	Path  string `json:"path"`
	Value int64  `json:"value"`
}

type ConfigMapInput struct {
	VcdHost   string
	ORG       string
	OVDC      string
	Network   string
	ClusterID string
	VAPP      string
}

func NewTestClient(host, org, userOrg, vdcName, username, token, clusterId string, getVdcClient bool) (*testingsdk.TestClient, error) {
	vcdAuthParams := &testingsdk.VCDAuthParams{
		Host:         host,
		OrgName:      org,
		UserOrg:      userOrg,
		OvdcName:     vdcName,
		Username:     username,
		RefreshToken: token,
		GetVdcClient: getVdcClient,
	}
	return testingsdk.NewTestClient(vcdAuthParams, clusterId)
}

func GetMapBySpecName(specMap map[string]interface{}, specName string, sectionName string) (map[string]interface{}, error) {
	entity, ok := specMap[specName]
	if !ok {
		return nil, fmt.Errorf("unable to get map: [%s] from %s in capiYaml String\n", specName, sectionName)
	}
	updatedSpecMap, ok := entity.(map[string]interface{})
	if !ok {
		return nil, fmt.Errorf("unable to convert [%T] to map[string]interface{}", entity)
	}
	return updatedSpecMap, nil
}
