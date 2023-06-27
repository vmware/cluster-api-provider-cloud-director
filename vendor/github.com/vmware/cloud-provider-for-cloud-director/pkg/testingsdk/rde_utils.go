package testingsdk

import (
	"context"
	"encoding/json"
	"fmt"
	"github.com/vmware/cloud-provider-for-cloud-director/pkg/vcdsdk"
)

// TODO: In the future, we will need to consider how to handle different versions of RDE. Currently these functions are not resilient to RDE version changes.

func GetVCDResourceSet(ctx context.Context, client *vcdsdk.Client, clusterId, componentName string) ([]vcdsdk.VCDResource, error) {
	vcdResourceSetMap, err := getVcdResourceSetComponentMapFromRDEId(ctx, client, clusterId, componentName)
	if err != nil {
		return nil, fmt.Errorf("error retrieving vcd resource set array from RDE [%s]: [%v]", clusterId, err)
	}
	return convertVcdResourceSetMapToVcdResourceArr(vcdResourceSetMap)
}

// Returns status.component as map[string]interface{}, this will help us narrow down to specific fields such as nodepools, vcdresources, etc
// Components: vcdKe, projector, csi, cpi, capvcd
func GetComponentMapInStatus(ctx context.Context, client *vcdsdk.Client, clusterId, componentName string) (map[string]interface{}, error) {
	rde, err := getRdeById(ctx, client, clusterId)
	if err != nil {
		return nil, fmt.Errorf("unable to get defined entity [%s]: [%v]", clusterId, err)
	}

	entityStatusIf, ok := rde.Entity["status"]
	if !ok {
		return nil, fmt.Errorf("status field not found in RDE [%s]", rde.Id)
	}

	entityStatus, ok := entityStatusIf.(map[string]interface{})
	if !ok {
		return nil, fmt.Errorf("failed to convert capvcd RDE [%s] status field from [%T] to map[string]interface{}",
			rde.Id, entityStatusIf)
	}

	componentStatusIf, ok := entityStatus[componentName]
	if !ok {
		return nil, fmt.Errorf("[%s] field not found in status field of RDE [%s]", componentName, rde.Id)
	}

	componentStatus, ok := componentStatusIf.(map[string]interface{})
	if !ok {
		return nil, fmt.Errorf("failed to convert [%s] status from [%T] to map[string]interface{} for RDE [%s]",
			componentName, componentStatusIf, clusterId)
	}
	return componentStatus, nil
}

func GetKubeconfigFromRDEId(ctx context.Context, client *vcdsdk.Client, clusterId string) (string, error) {
	capvcdStatusMap, err := GetComponentMapInStatus(ctx, client, clusterId, vcdsdk.ComponentCAPVCD)
	if err != nil {
		return "", fmt.Errorf("error retrieving [%s] field in status field of RDE [%s]: [%v]", vcdsdk.ComponentCAPVCD, clusterId, err)
	}

	privateStatusIf, ok := capvcdStatusMap["private"]
	if !ok {
		return "", fmt.Errorf("private field not found in status->capvcd of RDE [%s]", clusterId)
	}

	capvcdPrivateStatusMap, ok := privateStatusIf.(map[string]interface{})
	if !ok {
		return "", fmt.Errorf("failed to convert RDE [%s]  status->capvcd->private from [%T] to map[string]interface{}", clusterId, privateStatusIf)
	}

	kubeConfig, ok := capvcdPrivateStatusMap["kubeConfig"]
	if !ok {
		return "", fmt.Errorf("kubeConfig field not found in status->capvcd->private of RDE [%s]", clusterId)
	}
	return kubeConfig.(string), nil
}

func getVcdResourceSetComponentMapFromRDEId(ctx context.Context, client *vcdsdk.Client, clusterId, componentName string) (interface{}, error) {
	componentStatusMap, err := GetComponentMapInStatus(ctx, client, clusterId, componentName)
	if err != nil {
		return nil, fmt.Errorf("error retrieving field [%s] in status from RDE [%s]: [%v]", componentName, clusterId, err)
	}

	// Inferred type is interface{} for both key and values. So we cannot directly type assert into []vcdsdk.VCDResource.
	vcdResourceSetArr, ok := componentStatusMap[vcdsdk.ComponentStatusFieldVCDResourceSet]
	if !ok {
		return nil, fmt.Errorf("vcdResourceSet field not found in status->[%s] of RDE [%s]", componentName, clusterId)
	}
	return vcdResourceSetArr, nil
}

// This step is required because the type inferred is []interface{}, and we cannot do type assertion here, so we must marshal and unmarshal
func convertVcdResourceSetMapToVcdResourceArr(vcdResourceSetArr interface{}) ([]vcdsdk.VCDResource, error) {
	data, err := json.Marshal(vcdResourceSetArr)
	if err != nil {
		return nil, fmt.Errorf("error marshaling VCDResourceSet [%T]: [%v]", vcdResourceSetArr, err)
	}
	var vcdResourceSet []vcdsdk.VCDResource
	err = json.Unmarshal(data, &vcdResourceSet)
	if err != nil {
		return nil, fmt.Errorf("error unmarshaling data to [%T]: [%v]", vcdResourceSet, err)
	}
	return vcdResourceSet, nil
}

func getClusterNameById(ctx context.Context, client *vcdsdk.Client, clusterId string) (string, error) {
	rde, err := getRdeById(ctx, client, clusterId)
	if err != nil {
		return "", fmt.Errorf("unable to get defined entity by clusterId [%s]: [%v]", clusterId, err)
	}
	return rde.Name, nil
}
