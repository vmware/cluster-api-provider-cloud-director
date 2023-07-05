package utils

import (
	"fmt"
	"github.com/blang/semver"
	"github.com/vmware/cloud-provider-for-cloud-director/pkg/testingsdk"
	"github.com/vmware/go-vcloud-director/v2/types/v56"
	"strings"
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

		tkgVersionIsGreater, err := isGreaterThanOrEqual(tkgVersionStr, currTkgVersion)
		if err != nil {
			return nil, fmt.Errorf(
				"error running isGreaterThanOrEqual in %s and %s: [%v]", tkgVersionStr, currTkgVersion, err,
			)
		}

		if k8sUpgradeable && tkgVersionIsGreater {
			supportedUpgrades[k] = v
		}
	}
	return supportedUpgrades, nil
}

// getRDEUpgradeResources iterates through every vApp template installed in each catalog in the current org
// looking for a tkgOVA that matches one of the supported upgrade paths. When one is found, the resources
// needed to modify the capiyaml for a proper upgrade are returned.
// @param supportedUpgrades - a subset of tkg_map.json with all possible upgrade paths for the current cluster
// @param allVappTemplates - a list of every vApp template installed in every catalog in the current org
// @return string - key for the valid upgrade path in tkg_map.json. When modifying the capiyaml
// When modifying the capiyaml, we must have access to this key's values
// @return *types.QueryResultVappTemplateType - vApp Template instance of said valid upgrade
// When modifying the capiyaml, we will need to know some attributes of this vApp Template
// @return error - null if there is no error, the error otherwise
// [tkgVersion, k8sVersion]
func getRDEUpgradeResources(supportedUpgrades map[string]interface{}, allVappTemplates []*types.QueryResultVappTemplateType, targetK8sVersion, targetTkgVersion string) (string, *types.QueryResultVappTemplateType, error) {
	// Create a map to store the tkgOva data based on the k8s version
	tkgOvaMap := make(map[string]map[string]interface{})
	for _, v := range supportedUpgrades {
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
	}

	// Find the vAppTemplate matching the target k8s and TKG versions
	for _, vappTemplate := range allVappTemplates {
		k8sVersion := extractK8sVersionFromVappTemplateName(vappTemplate.Name)
		if k8sVersion == targetK8sVersion {
			tkgOva, ok := tkgOvaMap[k8sVersion]
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
				return k8sVersion, vappTemplate, nil
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

// UpgradeCluster is the one function that performs all the necessary steps and calls all the
// necessary functions to upgrade the current cluster
// @param testClient - test client used to access the cluster using common testing core
// @param currK8sVersion - the cluster's current k8s version
// @param tkgMap - tkg_map.json but marshalled into map[string]interface{}
// @param allVappTemplates - list of vApp Templates installed in each catalog in current org
// @return string - the updated capiyaml (for stringing operations, if necessary)
// @return string - the k8s version that we upgraded to
// @return error - null if there is no error, the error otherwise
func UpgradeCluster(testClient *testingsdk.TestClient, capiYaml, currK8sVersion, targetK8sVersion, targetTkgVersion string, tkgMap map[string]interface{}, allVappTemplates []*types.QueryResultVappTemplateType) (string, string, error) {

	currTkgVersion, err := getTkgVersionFromTKGMap(currK8sVersion, tkgMap)
	if err != nil {
		return "", "", fmt.Errorf("error retrieving TKG version from capiyaml: [%v]", err)
	}

	supportedUpgrades, err := getSupportedUpgrades(currK8sVersion, currTkgVersion, tkgMap)
	if err != nil {
		return "", "", fmt.Errorf("error getting supported upgrades for [%s]: [%v]", currK8sVersion, err)
	}
	tkgMapKey, vappTemplate, err := getRDEUpgradeResources(supportedUpgrades, allVappTemplates, targetK8sVersion, targetTkgVersion)
	if err != nil {
		return "", "", fmt.Errorf("error getting resources for modifying RDE to upgraded state: [%v]", err)
	}

	newCapiYaml, updatedK8sVersion, err := getCapiYamlAfterUpgrade(originalCapiYaml, tkgMapKey, tkgMap, vappTemplate)
	if err != nil {
		return "", "", fmt.Errorf("error constructing new capiyaml: [%v]", err)
	}

	err = replaceCapiYamlInRDE(testClient, newCapiYaml, clusterOrg)
	if err != nil {
		return newCapiYaml, updatedK8sVersion, fmt.Errorf("error replacing capiyaml in RDE in VCD: [%v]", err)
	}

	return newCapiYaml, updatedK8sVersion, nil
}
