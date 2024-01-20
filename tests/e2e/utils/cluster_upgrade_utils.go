package utils

import (
	"bufio"
	"bytes"
	"context"
	"fmt"
	"github.com/blang/semver"
	"github.com/vmware/cloud-provider-for-cloud-director/pkg/testingsdk"
	"github.com/vmware/go-vcloud-director/v2/types/v56"
	"io"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/util/wait"
	k8syaml "k8s.io/apimachinery/pkg/util/yaml"
	clusterv1beta1 "sigs.k8s.io/cluster-api/api/v1beta1"
	runtimeclient "sigs.k8s.io/controller-runtime/pkg/client"
	"strings"
	"time"
)

func getTkgVersionFromTKGMap(currK8sVersion string, tkgMap map[string]interface{}) (string, error) {
	for k, v := range tkgMap {
		// retrieving k8s version from tkr
		tkgOva, ok := v.(map[string]interface{})
		if !ok {
			return "", fmt.Errorf("error converting tkg_map unit into map[string]interface{}")
		}
		tkrIf, ok := tkgOva["tkr"]
		if !ok {
			return "", fmt.Errorf("tkr field not found in tkg_map unit")
		}
		tkr, ok := tkrIf.(string)
		tkrSplit := strings.Split(tkr, "---")
		if len(tkrSplit) < 2 {
			return "", fmt.Errorf("%s tkr version %s must be formatted like [k8s-version]---[something]", k, tkr)
		}
		k8sVersion := tkrSplit[0]
		if k8sVersion == currK8sVersion {
			// retrieving tkg version
			tkgVersionIf, ok := tkgOva["tkg"]
			if !ok {
				return "", fmt.Errorf("%v has no field [tkg]", tkgOva)
			}
			tkgVersion, ok := tkgVersionIf.([]interface{})
			if !ok {
				return "", fmt.Errorf("unable to convert %v to []interface{}", tkgVersionIf)
			}
			if len(tkgVersion) != 1 {
				return "", fmt.Errorf("%v must have 1 value", tkgVersion)
			}
			tkgVersionStr, ok := tkgVersion[0].(string)
			if !ok {
				return "", fmt.Errorf("unable to convert %v to string", tkgVersion[0])
			}
			return tkgVersionStr, nil
		}
	}
	return "", fmt.Errorf("unable to find the tkg Version")
}

// getSupportedUpgrades looks at tkg_map.json and returns a map of all possible upgrade paths
// based on the current cluster's tkg and k8s versions. An upgrade path is valid if the OVA's TKG version
// is greater or equal to the current cluster's TKG version and if the OVA's k8s version is 1 semver minor
// version greater than the current cluster's k8s version
// @param currK8sVersion - the cluster's current k8s version
// @param currTkgVersion - the cluster's current tkg version
// @param tkgMap - tkg_map.json, but marshalled into map[string]interface{}
// @return map[string]interface{} - map of all possible upgrade paths; it is a subset of tkgMap
// example ----- key: "v1.24.11+vmware.1-tkg.1-2ccb2a001f8bd8f15f1bfbc811071830" value: {
// "tkg": ["v2.2.0"], "tkr": "v1.24.11---vmware.1-tkg.1", "etcd": "v3.5.6_vmware.10", "coreDns": "v1.8.6_vmware.18" }
// @return error - null if there is no error, the error otherwise
func getSupportedUpgrades(currK8sVersion string, currTkgVersion string, tkgMap map[string]interface{}) (map[string]interface{}, error) {
	// for comparing k8s versions
	// comparing v1 to v2
	isOneMinorVersionGreater := func(v1 string, v2 string) (bool, error) {
		v1 = strings.TrimPrefix(v1, "v")
		v2 = strings.TrimPrefix(v2, "v")

		v1Semver, err := semver.New(v1)
		if err != nil {
			return false, fmt.Errorf("%s is an invalid semantic version string: [%v]", v1, err)
		}
		v2Semver, err := semver.New(v2)
		if err != nil {
			return false, fmt.Errorf("%s is an invalid semantic version string: [%v]", v1, err)
		}

		return (v1Semver.Major == v2Semver.Major) && (v1Semver.Minor == v2Semver.Minor+1), nil
	}

	// for comparing tkg versions
	// comparing v1 to v2
	isGreaterThanOrEqual := func(v1 string, v2 string) (bool, error) {
		v1 = strings.TrimPrefix(v1, "v")
		v2 = strings.TrimPrefix(v2, "v")

		v1Semver, err := semver.New(v1)
		if err != nil {
			return false, fmt.Errorf("%s is an invalid semantic version string: [%v]", v1, err)
		}
		v2Semver, err := semver.New(v2)
		if err != nil {
			return false, fmt.Errorf("%s is an invalid semantic version string: [%v]", v1, err)
		}

		return v1Semver.GTE(*v2Semver), nil
	}

	supportedUpgrades := make(map[string]interface{})

	for k, v := range tkgMap {
		// retrieving k8s version from tkr
		tkgOva, ok := v.(map[string]interface{})
		if !ok {
			return nil, fmt.Errorf("error converting tkg_map unit into map[string]interface{}")
		}
		tkrIf, ok := tkgOva["tkr"]
		if !ok {
			return nil, fmt.Errorf("tkr field not found in tkg_map unit")
		}
		tkr, ok := tkrIf.(string)
		tkrSplit := strings.Split(tkr, "---")
		if len(tkrSplit) < 2 {
			return nil, fmt.Errorf("%s tkr version %s must be formatted like [k8s-version]---[something]", k, tkr)
		}
		k8sVersion := tkrSplit[0]

		// retrieving tkg version
		tkgVersionIf, ok := tkgOva["tkg"]
		if !ok {
			return nil, fmt.Errorf("%v has no field [tkg]", tkgOva)
		}
		tkgVersion, ok := tkgVersionIf.([]interface{})
		if !ok {
			return nil, fmt.Errorf("unable to convert %v to []interface{}", tkgVersionIf)
		}
		if len(tkgVersion) != 1 {
			return nil, fmt.Errorf("%v must have 1 value", tkgVersion)
		}
		tkgVersionStr, ok := tkgVersion[0].(string)
		if !ok {
			return nil, fmt.Errorf("unable to convert %v to string", tkgVersion[0])
		}

		k8sUpgradeable, err := isOneMinorVersionGreater(k8sVersion, currK8sVersion)
		if err != nil {
			return nil, fmt.Errorf(
				"error checking isOneMinorVersionGreater in %s and %s: [%v]", currK8sVersion, k8sVersion, err,
			)
		}

		TkgVersionCompatible, err := isGreaterThanOrEqual(tkgVersionStr, currTkgVersion)
		if err != nil {
			return nil, fmt.Errorf(
				"error running isGreaterThanOrEqual in %s and %s: [%v]", tkgVersionStr, currTkgVersion, err,
			)
		}

		if k8sUpgradeable && TkgVersionCompatible {
			supportedUpgrades[k] = v
		}
	}
	return supportedUpgrades, nil
}

// getRDEUpgradeResources iterates through every vApp template installed in each catalog in the current org
// looking for a tkgOVA that matches one of the supported upgrade paths. When one is found, the resources
// needed to modify the capiyaml for a proper upgrade are returned.
// @param supportedUpgrades - a subset of tkg_map.json with all possible upgrade paths for the current cluster
// v1.24.11+vmware.1-tkg.1-2ccb2a001f8bd8f15f1bfbc811071830: {    "tkg": ["v2.2.0"], "tkr": "v1.24.11---vmware.1-tkg.1", "etcd": "v3.5.6_vmware.10", "coreDns": "v1.8.6_vmware.18" }
// @param allVappTemplates - a list of every vApp template installed in every catalog in the current org
// @return string - key for the valid upgrade path in tkg_map.json. When modifying the capiyaml
// When modifying the capiyaml, we must have access to this key's values
// @return *types.QueryResultVappTemplateType - vApp Template instance of said valid upgrade
// When modifying the capiyaml, we will need to know some attributes of this vApp Template
// @return error - null if there is no error, the error otherwise
// [tkgVersion, k8sVersion]
func getRDEUpgradeResources(supportedUpgrades map[string]interface{}, allVappTemplates []*types.QueryResultVappTemplateType, targetK8sVersion, targetTkgVersion string) (string, *types.QueryResultVappTemplateType, error) {
	// key-value pair (k8sVersion: tkgOvaInformation)
	// v1.24.11 : {    "tkg": ["v2.2.0"], "tkr": "v1.24.11---vmware.1-tkg.1", "etcd": "v3.5.6_vmware.10", "coreDns": "v1.8.6_vmware.18" }
	tkgOvaMap := make(map[string]map[string]interface{})
	// key-value pair (k8sVersion: tkgOvaName):
	// v1.24.11 : v1.24.11+vmware.1-tkg.1-2ccb2a001f8bd8f15f1bfbc811071830
	ovaNameMap := make(map[string]string)
	for k, v := range supportedUpgrades {
		tkgOva, ok := v.(map[string]interface{})
		if !ok {
			return "", nil, fmt.Errorf("error converting tkg_map unit into map[string]interface{}")
		}
		tkrIf, ok := tkgOva["tkr"]
		if !ok {
			return "", nil, fmt.Errorf("tkr field not found in tkg_map unit")
		}
		tkr, ok := tkrIf.(string)
		tkrSplit := strings.Split(tkr, "---")
		if len(tkrSplit) < 2 {
			return "", nil, fmt.Errorf("%s tkr version %s must be formatted like [k8s-version]---[something]", k, tkr)
		}
		k8sVersion := tkrSplit[0]
		tkgOvaMap[k8sVersion] = tkgOva
		ovaNameMap[k8sVersion] = k
	}

	// Find the vAppTemplate matching the target k8s and TKG versions
	for _, vappTemplate := range allVappTemplates {
		if strings.Contains(vappTemplate.Name, targetK8sVersion) {
			tkgOva, ok := tkgOvaMap[targetK8sVersion]
			if !ok {
				continue
			}
			tkgVersionIf, ok := tkgOva["tkg"]
			if !ok {
				return "", nil, fmt.Errorf("%v has no field [tkg]", tkgOva)
			}
			tkgVersion, ok := tkgVersionIf.([]interface{})
			if !ok {
				return "", nil, fmt.Errorf("unable to convert %v to []interface{}", tkgVersionIf)
			}
			if len(tkgVersion) != 1 {
				return "", nil, fmt.Errorf("%v must have 1 value", tkgVersion)
			}
			tkgVersionStr, ok := tkgVersion[0].(string)
			if !ok {
				return "", nil, fmt.Errorf("unable to convert %v to string", tkgVersion[0])
			}
			if tkgVersionStr == targetTkgVersion {
				return ovaNameMap[targetK8sVersion], vappTemplate, nil
			}
		}
	}

	return "", nil, fmt.Errorf("no vAppTemplate is installed with Kubernetes version: %s and TKG version: %s", targetK8sVersion, targetTkgVersion)
}

func extractK8sVersionFromVappTemplateName(templateName string) string {
	// Logic to extract the k8s version from the vAppTemplate name
	// Implement as per your naming convention
	// Example: Assuming the version is in the format "k8s-<version>"
	parts := strings.Split(templateName, "-")
	if len(parts) < 2 {
		return ""
	}
	return parts[1]
}

// ApplyK8sVersionUpgrade returns a new capiyaml with the target state that the cluster should have
// after upgrading to the specified TKG and Kubernetes version.
//
// Parameters:
// - capiYaml: The cluster's current capiyaml.
// - tkgMapKey: The key to a specific upgrade path in tkg_map.json.
// - tkgMap: The tkg_map.json marshalled into a map[string]interface{}.
// - vappTemplate: The vApp Template that matches the specific upgrade path.
//
// Returns:
// - string: The Kubernetes version that the cluster is being upgraded to.
// - error: nil if there is no error, otherwise the encountered error.
func ApplyK8sVersionUpgrade(ctx context.Context, r runtimeclient.Client, capiYaml string, tkgMapKey string, tkgMap map[string]interface{}, vappTemplate *types.QueryResultVappTemplateType) (string, error) {
	// processing all upgrade data from parameters that needs to be injected into capiyaml
	k8sVersion := strings.Split(tkgMapKey, "-")[0]

	tkgOva, err := testingsdk.GetMapBySpecName(tkgMap, tkgMapKey, tkgMapKey)
	if err != nil {
		return "", fmt.Errorf("error converting tkgMap.%s to map: [%v]", tkgMapKey, err)
	}

	coreDnsIf, ok := tkgOva["coreDns"]
	if !ok {
		return "", fmt.Errorf("%v has no field [coreDns]", tkgOva)
	}
	coreDns, ok := coreDnsIf.(string)
	if !ok {
		return "", fmt.Errorf("unable to convert %v to string", coreDnsIf)
	}

	etcdIf, ok := tkgOva["etcd"]
	if !ok {
		return "", fmt.Errorf("%v has no field [etcd]", tkgOva)
	}
	etcd, ok := etcdIf.(string)
	if !ok {
		return "", fmt.Errorf("unable to convert %v to string", etcdIf)
	}

	// injecting upgrade data into capiyaml
	hundredKB := 100 * 1024
	yamlReader := k8syaml.NewYAMLReader(bufio.NewReader(bytes.NewReader([]byte(capiYaml))))
	for err == nil {
		yamlBytes, err := yamlReader.Read()
		if err == io.EOF {
			break
		}
		yamlDecoder := k8syaml.NewYAMLOrJSONDecoder(bytes.NewReader(yamlBytes), hundredKB)
		unstructuredObj := unstructured.Unstructured{}
		err = yamlDecoder.Decode(&unstructuredObj)
		if err != nil {
			return "", fmt.Errorf("unable to parse yaml segment: [%v]\n", err)
		}

		kind := unstructuredObj.GetKind()
		switch kind {
		case KubeadmControlPlane:
			specMap, err := testingsdk.GetMapBySpecName(unstructuredObj.Object, "spec", "KubeadmControlPlane")
			if err != nil {
				return "", fmt.Errorf("error converting %s.spec to map: [%v]", KubeadmControlPlane, err)
			}
			specMap["version"] = k8sVersion

			kubeadmConfigSpecMap, err := testingsdk.GetMapBySpecName(
				specMap, "kubeadmConfigSpec", "KubeadmControlPlane.spec",
			)
			if err != nil {
				return "", fmt.Errorf("error converting %s.spec.kubeadmConfigSpec to map: [%v]", KubeadmControlPlane, err)
			}
			clusterConfigurationMap, err := testingsdk.GetMapBySpecName(
				kubeadmConfigSpecMap, "clusterConfiguration", "KubeadmControlPlane.spec.kubeadmConfigSpec",
			)
			if err != nil {
				return "", fmt.Errorf("error converting %s.spec.kubeadmConfigSpec.clusterConfiguration to map: [%v]", KubeadmControlPlane, err)
			}
			dnsMap, err := testingsdk.GetMapBySpecName(
				clusterConfigurationMap, "dns", "KubeadmControlPlane.spec.kubeadmConfigSpec.clusterConfiguration",
			)
			if err != nil {
				return "", fmt.Errorf("error converting %s.spec.kubeadmConfigSpec.clusterConfiguration.dns to map: [%v]", KubeadmControlPlane, err)
			}
			dnsMap["imageTag"] = coreDns

			etcdMap, err := testingsdk.GetMapBySpecName(
				clusterConfigurationMap, "etcd", "KubeadmControlPlane.spec.kubeadmConfigSpec.clusterConfiguration",
			)
			if err != nil {
				return "", fmt.Errorf("error converting %s.spec.kubeadmConfigSpec.clusterConfiguration.etcd to map: [%v]", KubeadmControlPlane, err)
			}
			localMap, err := testingsdk.GetMapBySpecName(
				etcdMap, "local", "KubeadmControlPlane.spec.kubeadmConfigSpec.clusterConfiguration.etcd",
			)
			if err != nil {
				return "", fmt.Errorf("error converting %s.spec.kubeadmConfigSpec.clusterConfiguration.etcd.local to map: [%v]", KubeadmControlPlane, err)
			}
			localMap["imageTag"] = etcd
			force := true
			executedErr := r.Patch(ctx, &unstructuredObj, runtimeclient.Apply, &runtimeclient.PatchOptions{
				Force:        &force,
				FieldManager: FieldManager,
			})

			if executedErr != nil {
				return "", fmt.Errorf("failed to patch %s: [%v]", KubeadmControlPlane, err)
			}

		case MachineDeployment:
			specMap, err := testingsdk.GetMapBySpecName(unstructuredObj.Object, "spec", "MachineDeployment")
			if err != nil {
				return "", fmt.Errorf("error converting %s.spec to map: [%v]", MachineDeployment, err)
			}
			templateMap, err := testingsdk.GetMapBySpecName(specMap, "template", "MachineDeployment.spec")
			if err != nil {
				return "", fmt.Errorf("error converting %s.spec.template to map: [%v]", MachineDeployment, err)
			}

			specMap2, err := testingsdk.GetMapBySpecName(templateMap, "spec", "MachineDeployment.spec.template")
			if err != nil {
				return "", fmt.Errorf("error converting %s.spec.template.spec to map: [%v]", MachineDeployment, err)
			}
			specMap2["version"] = k8sVersion

			force := true
			executedErr := r.Patch(ctx, &unstructuredObj, runtimeclient.Apply, &runtimeclient.PatchOptions{
				Force:        &force,
				FieldManager: FieldManager,
			})

			if executedErr != nil {
				return "", fmt.Errorf("failed to patch %s: [%v]", MachineDeployment, err)
			}

			break
		case VCDMachineTemplate:
			specMap, err := testingsdk.GetMapBySpecName(unstructuredObj.Object, "spec", "VCDMachineTemplate")
			if err != nil {
				return "", fmt.Errorf("error converting %s.spec to map: [%v]", VCDMachineTemplate, err)
			}
			templateMap, err := testingsdk.GetMapBySpecName(specMap, "template", "VCDMachineTemplate.spec")
			if err != nil {
				return "", fmt.Errorf("error converting %s.spec.template to map: [%v]", VCDMachineTemplate, err)
			}
			specMap2, err := testingsdk.GetMapBySpecName(templateMap, "spec", "VCDMachineTemplate.spec.template")
			if err != nil {
				return "", fmt.Errorf("error converting %s.spec.template.spec to map: [%v]", VCDMachineTemplate, err)
			}

			specMap2["catalog"] = vappTemplate.CatalogName
			specMap2["template"] = vappTemplate.Name

			force := true
			executedErr := r.Patch(ctx, &unstructuredObj, runtimeclient.Apply, &runtimeclient.PatchOptions{
				Force:        &force,
				FieldManager: FieldManager,
			})

			if executedErr != nil {
				return "", fmt.Errorf("failed to patch %s: [%v]", VCDMachineTemplate, err)
			}
		}
	}

	return k8sVersion, nil
}

// UpgradeCluster performs all the necessary steps and calls the required functions to upgrade the current cluster.
// It takes the following parameters:
// - ctx: the context for the operation
// - r: the runtime client used to access the cluster using common testing core
// - capiYaml: the current capiyaml
// - currK8sVersion: the cluster's current Kubernetes version
// - targetK8sVersion: the target Kubernetes version for the upgrade
// - targetTkgVersion: the target Tanzu Kubernetes Grid (TKG) version for the upgrade
// - tkgMap: tkg_map.json marshalled into map[string]interface{}
// - allVappTemplates: a list of vApp Templates installed in each catalog in the current organization
//
// The function returns the following values:
// - string: the updated capiyaml (for stringing operations, if necessary)
// - error: nil if there is no error, otherwise the encountered error
func UpgradeCluster(ctx context.Context, r runtimeclient.Client, capiYaml, currK8sVersion, targetK8sVersion, targetTkgVersion string, tkgMap map[string]interface{}, allVappTemplates []*types.QueryResultVappTemplateType) (string, error) {

	currTkgVersion, err := getTkgVersionFromTKGMap(currK8sVersion, tkgMap)
	if err != nil {
		return "", fmt.Errorf("error retrieving TKG version for a given k8s version [%s] from the tkgmap: [%v]", currK8sVersion, err)
	}
	//Todo: CAFV-297 getTKGInfoForAGivenK8sAndTKGVersion // get all the tkg details for a given K8s and tkg version; parses through entire tkg json and outputs the map (tkg details)
	supportedUpgrades, err := getSupportedUpgrades(currK8sVersion, currTkgVersion, tkgMap)
	if err != nil {
		return "", fmt.Errorf("error getting supported upgrades for [%s]: [%v]", currK8sVersion, err)
	}
	tkgMapKey, vappTemplate, err := getRDEUpgradeResources(supportedUpgrades, allVappTemplates, targetK8sVersion, targetTkgVersion)
	if err != nil {
		return "", fmt.Errorf("error getting resources for modifying RDE to upgraded state: [%v]", err)
	}

	updatedK8sVersion, err := ApplyK8sVersionUpgrade(ctx, r, capiYaml, tkgMapKey, tkgMap, vappTemplate)
	if err != nil {
		return "", fmt.Errorf("error applying the modified objects of KCP, MD, VCDMachineTemplate for upgrade operation: [%v]", err)
	}

	return updatedK8sVersion, nil
}

// MonitorK8sUpgrade polls the current cluster, checking if all nodes have been upgraded to a specified
// k8s version and then if all machines have reached running phase.
// @param testClient - test client needed to access the cluster using common testing core
// @param runtimeClient - k8s client needed to check if machines have reached running phase
// @param targetK8sVersion - the specified k8s version that we are checking for the nodes to be upgraded to
// @return error - null if there is no error, the error otherwise
func MonitorK8sUpgrade(
	testClient *testingsdk.TestClient, runtimeClient runtimeclient.Client, targetK8sVersion string,
) error {
	// timeout: 40 minutes; pollInterval: 2 minutes
	timeout := timeoutMinutes * time.Minute
	pollInterval := pollIntervalSeconds * time.Second

	return wait.PollImmediate(pollInterval, timeout, func() (bool, error) {
		// getting all nodes TODO CAFV-297: refract the code post 4.1 GA
		nodeList, err := testClient.GetNodes(context.Background())
		if err != nil {
			// when control plane is updating, we will get an error saying
			// "etcdserver request timeout" or
			// "etcdserver: leader changed" or
			// "the server was unable to return a response in the time allotted"
			// these are normal, so we have to count them as a retryable error
			if testingsdk.IsRetryableError(err) || strings.Contains(err.Error(), etcdServerRequestTimeoutErr) || strings.Contains(err.Error(), etcdServerLeaderChangedErr) || strings.Contains(err.Error(), serverTimeoutError) {
				fmt.Printf("RETRYABLE ERROR - error getting nodes: [%v]\n", err)
				fmt.Println("retrying monitor")
				return false, nil
			} else {
				return true, fmt.Errorf("UNRETRYABLE ERROR - error getting nodes [%v]", err)
			}
		}

		// checking for kubelet version for each node to reflect k8s version
		for _, node := range nodeList {
			nodeK8sVersion := node.Status.NodeInfo.KubeletVersion
			if nodeK8sVersion != targetK8sVersion {
				fmt.Printf(
					"[%s] does not match target k8s version %s in node %s\n", nodeK8sVersion, targetK8sVersion, node.Name,
				)
				fmt.Println("retrying monitor")
				return false, nil
			}
		}
		fmt.Println("k8sVersions Match!")

		// checking for all machines to reach running phase
		machineList := &clusterv1beta1.MachineList{}
		err = runtimeClient.List(context.Background(), machineList)
		if err != nil {
			return true, fmt.Errorf("error getting machine list [%v]", err)
		}

		for _, machine := range machineList.Items {
			if machine.Status.Phase != machinePhaseRunning {
				fmt.Printf("machine %s : phase: %s\n", machine.Name, machine.Status.Phase)
				fmt.Println("retrying monitor")
				return false, nil
			}
		}

		fmt.Println("all machines in running phase!")

		return true, nil
	})
}
