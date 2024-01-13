package testingsdk

import (
	"context"
	"encoding/json"
	"fmt"
	"github.com/blang/semver"
	"github.com/vmware/cloud-provider-for-cloud-director/pkg/vcdsdk"
	swaggerClient "github.com/vmware/cloud-provider-for-cloud-director/pkg/vcdswaggerclient_37_2"
	"github.com/vmware/go-vcloud-director/v2/govcd"
	"io"
	"net/http"
	"strings"
)

const (
	CAPVCDEntityTypeVersion120 = "1.2.0"
	NoOpDecryptBehaviorID      = "urn:vcloud:behavior-interface:getFullEntity:cse:capvcd:1.0.0"
)

type TaskWithResult struct {
	// NOTE: the following is the addition to the Task type in GoVCD. Represents the ResultContent within the Task object
	ResultContent string `xml:"Result>ResultContent,omitempty"`
}

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
	var kubeConfig string
	var err error

	clusterRde, err := getRdeById(ctx, client, clusterId)
	if err != nil {
		return "", fmt.Errorf("error occurred while getting cluster RDE [%s]: [%v]", clusterId, err)
	}

	entityTypeSemver, err := semver.New(getRDEVersion(clusterRde))
	if err != nil {
		return "", fmt.Errorf("error creating semver for retrieved RDE version from cluster [%s(%s)]: [%v]", clusterRde.Name, clusterId, err)
	}
	entityTypeVersion120, err := semver.New(CAPVCDEntityTypeVersion120)
	if err != nil {
		return "", fmt.Errorf("error creating semver from [%s] for comparison with RDE version: [%v]", CAPVCDEntityTypeVersion120, err)
	}

	// If RDE version of the cluster is >= 1.2.0, the kubeconfig will be stored as a secure field within the RDE's CAPVCD status
	// else, the kubeconfig can be directly obtained from the RDE

	// VCD/API version checks are not needed here because it is not possible to register RDE 1.2.0 for VCD builds 10.4.1 and below.
	// So all RDE versions are expected to be 1.1.0 from VCD 10.4.1 and below. It should not be possible to have a RDE 1.2.0 + VCD 10.3.x/10.4.1.
	// RDE 1.2.0 is only supported for VCD 10.4.2+, as UI will perform the API checks and register the appropriate schema.
	if entityTypeSemver.GE(*entityTypeVersion120) {
		kubeConfig, err = getKubeconfigFromDecryptedRDE(ctx, client, clusterId, clusterRde.Name)
	} else {
		kubeConfig, err = getKubeconfigFromRDE110(ctx, client, clusterId)
	}

	if err != nil {
		return "", fmt.Errorf("error occurred retrieving kubeconfig for cluster [%s(%s)] from RDE: [%v]", clusterRde.Name, clusterId, err)
	}

	if kubeConfig == "" {
		return "", fmt.Errorf("error occurred, empty kubeconfig was retrieved from cluster RDE [%s(%s)]", clusterRde.Name, clusterId)
	}
	return kubeConfig, err
}

func getDecryptedRDE(ctx context.Context, client *vcdsdk.Client, rdeID, clusterName string) (*FullCAPVCDEntity, error) {
	if client.VCDClient.Client.APIVCDMaxVersionIs(fmt.Sprintf("<%s", vcdsdk.VCloudApiVersion_37_2)) {
		return nil, fmt.Errorf("skipping decrypt RDE for cluster [%s(%s)] as VCD API version is less than [%s]", clusterName, rdeID, vcdsdk.VCloudApiVersion_37_2)
	}
	if client.APIClient == nil {
		return nil, fmt.Errorf("unable to decrypt RDE for cluster [%s(%s)] as API client for VCD API version [%s] is missing", clusterName, rdeID, vcdsdk.VCloudApiVersion_37_2)
	}

	resp, err := client.APIClient.DefinedInterfaceBehaviorsApi.InvokeDefinedEntityBehavior(ctx, rdeID, NoOpDecryptBehaviorID, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to call decrypt behavior on RDE for cluster [%s(%s)]: [%v]", clusterName, rdeID, err)
	}
	if resp == nil {
		return nil, fmt.Errorf("obtained nil response for the API call to decrypt behavior of RDE for the cluster [%s(%s)]", clusterName, rdeID)
	}
	if resp.StatusCode != http.StatusAccepted {
		respBytes, err := io.ReadAll(resp.Body)
		if err == nil {
			return nil, fmt.Errorf("obtained error response with status code [%d] for the API call to decrypt behavior of RDE for cluster [%s(%s)]: [%s]", resp.StatusCode, clusterName, rdeID, string(respBytes))
		}
		return nil, fmt.Errorf("obtained unexpected response code [%d] for the API call to decrypt behavior of RDE for cluster [%s(%s)]", resp.StatusCode, clusterName, rdeID)
	}

	decryptEntityTaskUrl := resp.Header.Get("Location")
	if decryptEntityTaskUrl == "" {
		return nil, fmt.Errorf("unexpected response for decrypt behavior of RDE for cluster [%s(%s)] - task URL is empty", clusterName, rdeID)
	}
	task := govcd.NewTask(&client.VCDClient.Client)
	task.Task.HREF = resp.Header.Get("Location")
	err = task.WaitTaskCompletion()
	if err != nil {
		return nil, fmt.Errorf("task for enitity decryption failed for RDE for cluster [%s(%s)]: [%v]", clusterName, rdeID, err)
	}

	// The following request to get the task needs to be explicitly made because GoVCD type for Task doesn't parse the Result field in the response.
	taskResult := &TaskWithResult{}
	_, err = client.VCDClient.Client.ExecuteRequest(decryptEntityTaskUrl, http.MethodGet, "", "error getting task: %s", nil, taskResult)
	if err != nil {
		return nil, fmt.Errorf("failed to read task response after decrypting RDE for cluster [%s(%s)]: [%v]", clusterName, rdeID, err)
	}
	if taskResult.ResultContent == "" {
		return nil, fmt.Errorf("unexpected error - decrypted task is empty for RDE for cluster [%s(%s)]", clusterName, rdeID)
	}

	// parse the ResultContent in the task into capvcdFullDefinedEntity type structure
	capvcdFullRDE := &FullCAPVCDEntity{}
	if err := json.Unmarshal([]byte(taskResult.ResultContent), capvcdFullRDE); err != nil {
		return nil, fmt.Errorf("failed to parse decrypted entity into CAPVCD struct for RDE for cluster [%s(%s)]: [%v]", clusterName, rdeID, err)
	}
	return capvcdFullRDE, nil
}

func getKubeconfigFromCapvcdStatus(capvcdStatusMap map[string]interface{}, clusterId string) (string, error) {
	if capvcdStatusMap == nil {
		return "", fmt.Errorf("capvcd status map is nil for cluster [%s]", clusterId)
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

func getKubeconfigFromRDE110(ctx context.Context, client *vcdsdk.Client, clusterId string) (string, error) {
	capvcdStatusMap, err := GetComponentMapInStatus(ctx, client, clusterId, vcdsdk.ComponentCAPVCD)
	if err != nil {
		return "", fmt.Errorf("error retrieving [%s] field in status field of RDE [%s]: [%v]", vcdsdk.ComponentCAPVCD, clusterId, err)
	}
	return getKubeconfigFromCapvcdStatus(capvcdStatusMap, clusterId)
}

func getKubeconfigFromDecryptedRDE(ctx context.Context, client *vcdsdk.Client, clusterId, clusterName string) (string, error) {
	fullCapvcdRde, err := getDecryptedRDE(ctx, client, clusterId, clusterName)
	if err != nil {
		return "", fmt.Errorf("error retrieving kubeconfig for cluster [%s(%s)] from decrypted RDE: [%v]", clusterName, clusterId, err)
	}

	capvcdStatusMap := fullCapvcdRde.Entity.Status.CAPVCDStatus
	return getKubeconfigFromCapvcdStatus(capvcdStatusMap, clusterId)
}

func getRDEVersion(rde *swaggerClient.DefinedEntity) string {
	entiyTypeSplitArr := strings.Split(rde.EntityType, ":")
	// last item of the array will be the version string
	return entiyTypeSplitArr[len(entiyTypeSplitArr)-1]
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
