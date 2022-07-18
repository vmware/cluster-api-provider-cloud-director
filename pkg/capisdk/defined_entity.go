package capisdk

import (
	"context"
	"encoding/json"
	"fmt"
	"github.com/pkg/errors"
	"github.com/vmware/cloud-provider-for-cloud-director/pkg/vcdsdk"
	swagger "github.com/vmware/cloud-provider-for-cloud-director/pkg/vcdswaggerclient"
	"github.com/vmware/cluster-api-provider-cloud-director/pkg/util"
	"github.com/vmware/cluster-api-provider-cloud-director/pkg/vcdtypes/rde_type_1_0_0"
	rdeType "github.com/vmware/cluster-api-provider-cloud-director/pkg/vcdtypes/rde_type_1_1_0"
	"github.com/vmware/cluster-api-provider-cloud-director/release"
	"k8s.io/klog"
	"net/http"
	"net/url"
	"reflect"
	"strings"
	"time"
)

const (
	MaxUpdateRetries = 10

	StatusComponentNameCAPVCD = "cluster-api-provider-cloud-director"

	CAPVCDTypeVendor       = "vmware"
	CAPVCDTypeNss          = "capvcdCluster"
	CAPVCDEntityTypePrefix = "urn:vcloud:type:vmware:capvcdCluster"

	CAPVCDClusterKind             = "CAPVCDCluster"
	CAPVCDClusterEntityApiVersion = "capvcd.vmware.com/v1.1"
	DefaultRollingWindowSize      = 20

	// VCDCluster Events
	RdeAvailable          = "RdeAvailable"
	RdeUpgraded           = "RdeUpgraded"
	LoadBalancerAvailable = "LoadBalancerAvailable"
	InfraVappAvailable    = "InfraVappAvailable"
	ControlplaneReady     = "ControlplaneReady"
	LoadbalancerDeleted   = "LoadbalancerDeleted"
	VappDeleted           = "vAppDeleted"

	// VCDCluster Errors
	RdeUpgradeError         = "RdeUpgradeError"
	LoadbalancerPending     = "LoadBalancerPending"
	VappCreationError       = "vAppCreationError"
	LoadBalancerDeleteError = "LoadBalancerDeleteError"
	VappDeleteError         = "VappDeleteError"
	RdeDeleteError          = "RdeDeleteError"

	// VCDMachine Events
	InfraVmPoweredOn         = "VcdMachineInfraVMPoweredOn"
	CloudInitScriptGenerated = "VcdMachineBootstrapScriptGenerated"
	InfraVmBootstrapped      = "VcdMachineBootstrapped"
	InfraVmDeleted           = "VcdMachineInfraVmDeleted"

	// VCDMachine Errors
	ScriptGenerationError = "VcdMachineScriptGenerationError"
	InfraVMCreationError  = "VcdMachineInfraVMCreationError"
	ScriptExecutionError  = "VcdMachineScriptExecutionError"
	InfraVmDeleteError    = "VcdMachineInfraVmDeleteError"
)

// During upgrade from any old rde to a newer version format, we must be careful not to wipe out
// sections maintained by other components like CSI, CPI, VKP.
// Those sections can have their own upgrade logic independent of capvcd's upgrades.
// Below is the retain list of the sections in "status" that CAPVCD should not touch.
var (
	sectionsInStatusRetainedDuringRDEUpgrade = []string{
		"persistentVolumes",
		"virtualIPs",
		"vkp",
		"cpi",
		"csi",
	}
)

type CapvcdRdeManager struct {
	Client     *vcdsdk.Client
	RdeManager *vcdsdk.RDEManager
}

func NewCapvcdRdeManager(client *vcdsdk.Client, clusterID string) *CapvcdRdeManager {
	return &CapvcdRdeManager{
		Client: client,
		RdeManager: &vcdsdk.RDEManager{
			Client:                 client,
			StatusComponentName:    StatusComponentNameCAPVCD,
			StatusComponentVersion: release.CAPVCDVersion,
			ClusterID:              clusterID,
		},
	}
}

func convertToMap(obj interface{}) (map[string]interface{}, error) {
	byteArr, err := json.Marshal(obj)
	if err != nil {
		return nil, fmt.Errorf("failed to read object [%v] to json: [%v]", obj, err)
	}
	var parsedMap map[string]interface{}
	if err := json.Unmarshal(byteArr, &parsedMap); err != nil {
		return nil, fmt.Errorf("failed to convert byte array [%s] to map[string]interface{}: [%v]", string(byteArr), err)
	}
	return parsedMap, nil
}

func patchObject(inputObj interface{}, patchMap map[string]interface{}) (map[string]interface{}, error) {
	for k, v := range patchMap {
		fields := strings.Split(k, ".")
		updatedVal := reflect.ValueOf(v)
		typ := reflect.TypeOf(inputObj)
		klog.V(4).Infof("Assigning value %v to key %s in [%v]", v, k, typ)
		objVal := reflect.ValueOf(inputObj).Elem()
		for _, attr := range fields {
			// cannot call fieldByName on a zero value
			objVal = objVal.FieldByName(attr)
			if objVal.Kind() == reflect.Ptr {
				objVal = objVal.Elem()
			}
		}
		objVal.Set(updatedVal)
	}
	updatedMap, err := convertToMap(inputObj)
	if err != nil {
		return nil, fmt.Errorf("failed to convert object [%v] to map[string]interface{}: [%v]", inputObj, err)
	}
	return updatedMap, nil
}

// SetIsManagementClusterInRDE : sets the isManagementCluster flag in RDE for the management cluster
func (capvcdRdeManager *CapvcdRdeManager) SetIsManagementClusterInRDE(ctx context.Context, managementClusterRDEId string) error {
	if managementClusterRDEId == "" {
		klog.V(3).Infof("RDE ID for the management cluster not found. Skip setting isManagementCluster flag for the RDE.")
		return nil
	}
	capvcdStatusPatch := make(map[string]interface{})
	capvcdStatusPatch["UsedAsManagementCluster"] = true
	_, err := capvcdRdeManager.PatchRDE(ctx, nil, nil, capvcdStatusPatch, managementClusterRDEId)
	if err != nil {
		return fmt.Errorf("failed to set isManagementCluster flag for management cluster with RDE ID [%s]: [%v]",
			managementClusterRDEId, err)
	}
	return nil
}

// PatchRDE : Update only specific fields in the RDE. Takes in a map with keys, which contain "." delimitted
// strings, representing the spec, metadata and cavcd status fields to be updated.
// Example: To patch only the "spec.capiYaml", "metadata.name", "status.capvcd.version" portion of the RDE, specPatch map should be something like this -
// specPatch["CapiYaml"] = updated-yaml
// metadataPatch["Name"] = updated-name
// capvcdStatusPatch["Version"] = updated-version
func (capvcdRdeManager *CapvcdRdeManager) PatchRDE(ctx context.Context, specPatch, metadataPatch, capvcdStatusPatch map[string]interface{}, rdeID string) (rde *swagger.DefinedEntity, err error) {
	defer func() {
		// recover from panic if panic occurs because of
		// 1. calling Set() on a zero value
		if r := recover(); r != nil {
			klog.Errorf("panic occurred while patching RDE: [%v]", r)
			err = errors.Errorf("recovered panic during updating entity: [%v]", r)
		}
	}()
	client := capvcdRdeManager.Client
	for retries := 0; retries < MaxUpdateRetries; retries++ {
		rde, resp, etag, err := client.APIClient.DefinedEntityApi.GetDefinedEntity(ctx, rdeID)
		if err != nil {
			return nil, fmt.Errorf("failed to call get defined entity RDE with ID [%s]: [%s]", rdeID, err)
		}
		if len(specPatch) == 0 && len(metadataPatch) == 0 && len(capvcdStatusPatch) == 0 {
			// no updates to the entity
			return &rde, nil
		}

		if resp.StatusCode != http.StatusOK {
			return nil, fmt.Errorf("error getting the defined entity with ID [%s]", rdeID)
		}

		capvcdEntity, err := util.ConvertMapToCAPVCDEntity(rde.Entity)
		if err != nil {
			return nil, fmt.Errorf("failed to convert map to CAPVCD entity [%v]", err)
		}

		// patch entity.spec portion of the CAPVCD RDE
		if len(specPatch) != 0 {
			rde.Entity["spec"], err = patchObject(&capvcdEntity.Spec, specPatch)
			if err != nil {
				return nil, fmt.Errorf("failed to patch spec of CAPVCD entity: [%v]", err)
			}
		}

		// patch entity.metadata portion of the CAPVCD RDE
		if len(metadataPatch) != 0 {
			rde.Entity["metadata"], err = patchObject(&capvcdEntity.Metadata, metadataPatch)
			if err != nil {
				return nil, fmt.Errorf("failed to patch metadata of CAPVCD entity: [%v]", err)
			}
		}

		// patch entity.status.capvcd portion of the CAPVCD RDE
		if len(capvcdStatusPatch) != 0 {
			// fetch the CAPVCD status from the RDE
			statusIf, ok := rde.Entity["status"]
			if !ok {
				return nil, fmt.Errorf("error parsing status section of CAPVCD entity")
			}
			statusMap, ok := statusIf.(map[string]interface{})
			if !ok {
				return nil, fmt.Errorf("error parsing status section of CAPVCD entity to map[string]interface{}")
			}
			statusMap["capvcd"], err = patchObject(&capvcdEntity.Status.CAPVCDStatus, capvcdStatusPatch)
			if err != nil {
				return nil, fmt.Errorf("failed to patch capvcd status in the CAPVCD entity: [%v]", err)
			}
		}

		// update the defined entity
		rde, resp, err = client.APIClient.DefinedEntityApi.UpdateDefinedEntity(ctx, rde, etag, rdeID, nil)
		if err != nil {
			klog.Errorf("failed to update defined entity with ID [%s]: [%v]. Remaining retry attempts: [%d]", rdeID, err, MaxUpdateRetries-retries-1)
			continue
		}
		if resp.StatusCode != http.StatusOK {
			klog.Errorf("error updating the defined entity with ID [%s]. failed with status code [%d]. Remaining retry attempts: [%d]", rdeID, resp.StatusCode, MaxUpdateRetries-retries+1)
			continue
		}
		klog.V(4).Infof("successfully updated defined entity with ID [%s]", rdeID)
		return &rde, nil
	}
	return nil, fmt.Errorf("failed to update defined entity with ID [%s]", rdeID)
}

// GetCAPVCDEntity parses CAPVCDStatus and CAPVCD
func (capvcdRdeManager *CapvcdRdeManager) GetCAPVCDEntity(ctx context.Context, rdeID string) (*swagger.DefinedEntity, *rdeType.CAPVCDSpec, *rdeType.Metadata, *rdeType.CAPVCDStatus, error) {
	client := capvcdRdeManager.Client
	rde, resp, _, err := client.APIClient.DefinedEntityApi.GetDefinedEntity(ctx, rdeID)
	if err != nil {
		return nil, nil, nil, nil, fmt.Errorf("failed to get defined entity with ID [%s]: [%v]", rdeID, err)
	}
	if resp.StatusCode != http.StatusOK {
		return nil, nil, nil, nil, fmt.Errorf("error getting defined entity with ID [%s]: [%v]", rdeID, err)
	}
	capvcdEntity, err := util.ConvertMapToCAPVCDEntity(rde.Entity)
	if err != nil {
		return nil, nil, nil, nil, fmt.Errorf("failed to convert CAPVCD entity map to type CAPVCD entity: [%v]", err)
	}
	return &rde, &capvcdEntity.Spec, &capvcdEntity.Metadata, &capvcdEntity.Status.CAPVCDStatus, nil
}

func (capvcdRdeManager *CapvcdRdeManager) GetRDEVersion(ctx context.Context, rdeID string) (*swagger.DefinedEntity, string, error) {
	definedEntity, resp, _, err := capvcdRdeManager.Client.APIClient.DefinedEntityApi.GetDefinedEntity(ctx, rdeID)
	if err != nil {
		var responseMessageBytes []byte
		if gsErr, ok := err.(swagger.GenericSwaggerError); ok {
			responseMessageBytes = gsErr.Body()
			klog.Errorf("error occurred when getting RDE type version for defined entity [%s]: [%s]",
				rdeID, string(responseMessageBytes))
		}
		return nil, "", fmt.Errorf("failed to get RDE type version for defined entity [%s]", rdeID)
	} else if resp == nil {
		return nil, "", fmt.Errorf("unexpected response when fetching the RDE type version for RDE [%s]; obtained nil response",
			rdeID)
	} else {
		if resp.StatusCode != http.StatusOK {
			return nil, "", fmt.Errorf("obtained unexpected response status code [%d] when fetching the RDE version for entity [%s]. expected [%d]",
				resp.StatusCode, rdeID, http.StatusOK)
		}
	}

	entiyTypeSplitArr := strings.Split(definedEntity.EntityType, ":")
	// last item of the array will be the version string
	return &definedEntity, entiyTypeSplitArr[len(entiyTypeSplitArr)-1], nil
}

// EntityType contains only the required properties in get entity type response
type EntityType struct {
	ID      string                 `json:"id"`
	Name    string                 `json:"name"`
	Nss     string                 `json:"nss"`
	Version string                 `json:"version"`
	Schema  map[string]interface{} `json:"schema"`
}

func (capvcdRdeManager *CapvcdRdeManager) IsCapvcdEntityTypeRegistered(version string) bool {
	entityTypeID := strings.Join(
		[]string{
			CAPVCDEntityTypePrefix,
			version,
		},
		":",
	)
	entityTypeListUrl, err := capvcdRdeManager.Client.VCDClient.Client.OpenApiBuildEndpoint(
		"1.0.0/entityTypes/" + entityTypeID)
	if err != nil {
		klog.Errorf("failed to construct URL to get list of entity types")
		return false
	}
	var output EntityType
	err = capvcdRdeManager.Client.VCDClient.Client.OpenApiGetItem(
		capvcdRdeManager.Client.VCDClient.Client.APIVersion, entityTypeListUrl,
		url.Values{}, &output, nil)
	if err != nil {
		klog.Errorf("CAPVCD entity type [%s] not registered: [%v]", entityTypeID, err)
		return false
	}
	klog.V(4).Info("Found CAPVCD entity type")
	return true
}

func (capvcdRdeManager *CapvcdRdeManager) convertFrom100Format(ctx context.Context, srcRde *swagger.DefinedEntity, srcRdeTypeVersion string) (*swagger.DefinedEntity, error) {
	if srcRdeTypeVersion == rdeType.CapvcdRDETypeVersion {
		klog.V(4).Infof("RDE [%s] is already upgraded to version [%s]", srcRde.Id, rdeType.CapvcdRDETypeVersion)
		return srcRde, nil
	}
	for retries := 0; retries < MaxUpdateRetries; retries++ {
		srcCapvcdEntity, resp, etag, err := capvcdRdeManager.Client.APIClient.DefinedEntityApi.GetDefinedEntity(ctx, srcRde.Id)
		if err != nil {
			var responseMessageBytes []byte
			if gsErr, ok := err.(swagger.GenericSwaggerError); ok {
				responseMessageBytes = gsErr.Body()
				klog.Errorf("error occurred when upgrading defined entity [%s] to version [%s]: [%s]",
					srcRde.Id, rdeType.CapvcdRDETypeVersion, string(responseMessageBytes))
			}
			return nil, fmt.Errorf("failed to get the defined entity to convert RDE to version [%s]", rdeType.CapvcdRDETypeVersion)
		} else if resp == nil {
			return nil, fmt.Errorf("unexpected response when fetching the defined entity [%s] to version [%s]; obtained nil response",
				srcRde.Id, rdeType.CapvcdRDETypeVersion)
		} else {
			if resp.StatusCode != http.StatusOK {
				return nil, fmt.Errorf("obtained unexpected response status code [%d] when fetching the defined entity [%s]. expected [%d]",
					resp.StatusCode, srcRde.Id, http.StatusOK)
			}
		}

		dstEmptyCapvcdEntity := rdeType.CAPVCDEntity{
			Kind: CAPVCDClusterKind,
			Spec: rdeType.CAPVCDSpec{
				CapiYaml: "", // will be eventually updated by CAPVCD
			},
			ApiVersion: CAPVCDClusterEntityApiVersion,
		}
		dstCapvcdEntityMap, err := util.ConvertCAPVCDEntityToMap(&dstEmptyCapvcdEntity)
		if err != nil {
			return nil, fmt.Errorf("failed to convert map[string]interface{} to CAPVCD entity object: [%v]", err)
		}

		var newStatusMap map[string]interface{}
		newStatusMap = nil
		if srcEntityStatus, ok := srcCapvcdEntity.Entity["status"]; ok {
			if statusMap, ok := srcEntityStatus.(map[string]interface{}); ok {
				newStatusMap = make(map[string]interface{})
				for _, s := range sectionsInStatusRetainedDuringRDEUpgrade {
					if oldSectionValue, ok := statusMap[s]; ok {
						newStatusMap[s] = oldSectionValue
					}
				}
			}
		}
		if newStatusMap == nil {
			klog.V(4).Infof("entity [%s] doesn't contain status section. Not possible to retain [%#v] parts of status section in the upgraded RDE.",
				srcRde.Id, sectionsInStatusRetainedDuringRDEUpgrade)
		}

		dstCapvcdEntityMap["status"] = newStatusMap

		dstCapvcdRde := swagger.DefinedEntity{
			EntityType: CAPVCDEntityTypePrefix + ":" + rdeType.CapvcdRDETypeVersion,
			Entity:     dstCapvcdEntityMap,
			Name:       srcRde.Name,
			ExternalId: srcRde.ExternalId,
		}

		updatedRde, resp, err := capvcdRdeManager.Client.APIClient.DefinedEntityApi.UpdateDefinedEntity(
			ctx, dstCapvcdRde, etag, srcRde.Id, nil)
		if err != nil {
			var responseMessageBytes []byte
			if gsErr, ok := err.(swagger.GenericSwaggerError); ok {
				responseMessageBytes = gsErr.Body()
				klog.Errorf("error occurred when upgrading defined entity [%s] to version [%s]: [%s]",
					srcRde.Id, rdeType.CapvcdRDETypeVersion, string(responseMessageBytes))
			}
			return nil, fmt.Errorf("error when upgrading defined entity [%s] to version [%s]: [%v]",
				srcRde.Id, rdeType.CapvcdRDETypeVersion, err)
		} else if resp == nil {
			return nil, fmt.Errorf("unexpected response when upgrading defined entity [%s] to version [%s]; obtained nil response",
				srcRde.Id, rdeType.CapvcdRDETypeVersion)
		} else {
			if resp.StatusCode == http.StatusPreconditionFailed {
				klog.Errorf("wrong etag found when upgrading the defined entity [%s] to version [%s]. Retries remaining: [%d]",
					srcRde.Id, rdeType.CapvcdRDETypeVersion, MaxUpdateRetries-retries-1)
				continue
			} else if resp.StatusCode != http.StatusOK {
				return nil, fmt.Errorf("unexpected response status code when upgrading defined entity [%s] to version [%s]. Expected response [%d] obtained [%d]",
					srcRde.Id, rdeType.CapvcdRDETypeVersion, http.StatusOK, resp.StatusCode)
			}
		}
		klog.V(4).Infof("successfully upgraded RDE [%s] to version [%s]", srcRde.Id, rdeType.CapvcdRDETypeVersion)
		return &updatedRde, nil
	}

	return nil, fmt.Errorf("failed to upgrade RDE [%s] to version [%s] after [%d] retries",
		srcRde.Id, rdeType.CapvcdRDETypeVersion, MaxUpdateRetries)
}

// ConvertToLatestRDEVersionFormat updates the RDE version. The upgraded RDE will only contain minimal information related to the cluster after upgrade.
//  CAPVCD will reconcile the RDE eventually with proper data by CAPVCD.
//  The function attempts upgrade multiple times as defined by MaxUpdateRetries to avoid failures due to incorrect ETag.
// 	"entity.status.persistentVolumes" and "entity.status.virtualIPs" in the existing RDE will be retained in the upgraded RDE.
func (capvcdRdeManager *CapvcdRdeManager) ConvertToLatestRDEVersionFormat(ctx context.Context, rdeID string) (*swagger.DefinedEntity, error) {
	if !capvcdRdeManager.IsCapvcdEntityTypeRegistered(rdeType.CapvcdRDETypeVersion) {
		return nil, fmt.Errorf("CAPVCD entity type with version [%s] not registered", rdeType.CapvcdRDETypeVersion)
	}
	srcRde, srcRdeTypeVersion, err := capvcdRdeManager.GetRDEVersion(ctx, rdeID)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch RDE type version for RDE [%s]", rdeID)
	}

	var dstRde *swagger.DefinedEntity
	switch srcRdeTypeVersion {
	case rde_type_1_0_0.CapvcdRDETypeVersion:
		dstRde, err = capvcdRdeManager.convertFrom100Format(ctx, srcRde, srcRdeTypeVersion)
	default:
		err = fmt.Errorf("failed to upgrade RDE [%s]; invalid source RDE version [%s]", rdeID, srcRdeTypeVersion)
	}
	if err != nil {
		return nil, fmt.Errorf("failed to convert RDE [%s] from source version [%s] to destination version [%s]",
			srcRde.Id, srcRde.EntityType, rdeType.CapvcdRDETypeVersion)
	}

	return dstRde, nil
}

func (capvcdRdeManager *CapvcdRdeManager) AddToErrorSet(ctx context.Context, errorName, vcdResourceId, vcdResourceName, detailedErrorMsg string) error {
	backendErr := vcdsdk.BackendError{
		Name:            errorName,
		OccurredAt:      time.Now(),
		VcdResourceId:   vcdResourceId,
		VcdResourceName: vcdResourceName,
	}
	if detailedErrorMsg != "" {
		backendErr.AdditionalDetails = map[string]interface{}{"error": detailedErrorMsg}
	}
	return capvcdRdeManager.RdeManager.AddToErrorSet(ctx, vcdsdk.ComponentCAPVCD, backendErr, DefaultRollingWindowSize)
}

func (capvcdRdeManager *CapvcdRdeManager) AddToEventSet(ctx context.Context, eventName, vcdResourceId, vcdResourceName, detailedEventMsg string) error {
	backendEvent := vcdsdk.BackendEvent{
		Name:            eventName,
		OccurredAt:      time.Now(),
		VcdResourceId:   vcdResourceId,
		VcdResourceName: vcdResourceName,
	}
	if detailedEventMsg != "" {
		backendEvent.AdditionalDetails = map[string]interface{}{"event": detailedEventMsg}
	}
	return capvcdRdeManager.RdeManager.AddToEventSet(ctx, vcdsdk.ComponentCAPVCD, backendEvent, DefaultRollingWindowSize)
}
