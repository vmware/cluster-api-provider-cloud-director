package utils

import "github.com/vmware/cloud-provider-for-cloud-director/pkg/testingsdk"

const (
	timeoutMinutes          = 40
	pollIntervalSeconds     = 120
	machinePhaseProvisioned = "Provisioned"
	machinePhaseRunning     = "Running"
	VCDCluster              = "VCDCluster"
	Cluster                 = "Cluster"
	SECRET                  = "Secret"
	KubeadmControlPlane     = "KubeadmControlPlane"
	VCDMachineTemplate      = "VCDMachineTemplate"
	KubeadmConfigTemplate   = "KubeadmConfigTemplate"
	MachineDeployment       = "MachineDeployment"
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
