package utils

import (
	"fmt"
	"github.com/vmware/cloud-provider-for-cloud-director/pkg/testingsdk"
	"strings"
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

// injectValues replaces placeholders in the provided data map with the corresponding values from the replacements map.
func injectValues(data map[string]string, replacements map[string]string) map[string]string {
	injectedData := make(map[string]string)
	for key, value := range data {
		injectedValue := value
		for placeholder, replacement := range replacements {
			placeholderKey := fmt.Sprintf("{{%s}}", placeholder)
			injectedValue = strings.ReplaceAll(injectedValue, placeholderKey, replacement)
		}
		injectedData[key] = injectedValue
	}
	return injectedData
}
