package vcdclient

import (
	"context"
	"encoding/json"
	"fmt"
	"github.com/pkg/errors"
	"github.com/vmware/cluster-api-provider-cloud-director/pkg/util"
	vcdutil "github.com/vmware/cluster-api-provider-cloud-director/pkg/util"
	swagger "github.com/vmware/cluster-api-provider-cloud-director/pkg/vcdswaggerclient"
	rdeType "github.com/vmware/cluster-api-provider-cloud-director/pkg/vcdtypes/rde_type_1_1_0"
	"k8s.io/klog"
	"net/http"
	"reflect"
	"strings"
)

const (
	MaxUpdateRetries = 10
)

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

// SetIsManagementClusterInRDE: sets the isManagementCluster flag in RDE for the management cluster
func (client *Client) SetIsManagementClusterInRDE(ctx context.Context) error {
	if client.ManagementClusterRDEId == "" {
		klog.V(3).Infof("RDE ID for the management cluster not found. Skip setting isManagementCluster flag for the RDE.")
		return nil
	}
	capvcdStatusPatch := make(map[string]interface{})
	capvcdStatusPatch["UsedAsManagementCluster"] = true
	_, err := client.PatchRDE(ctx, nil, nil, capvcdStatusPatch, client.ManagementClusterRDEId)
	if err != nil {
		return fmt.Errorf("failed to set isManagementCluster flag for management cluster with RDE ID [%s]: [%v]", client.ManagementClusterRDEId, err)
	}
	return nil
}

// PatchRDE: Update only specific fields in the RDE. Takes in a map with keys, which contain "." delimitted
// strings, representing the spec, metadata and cavcd status fields to be updated.
// Example: To patch only the "spec.capiYaml", "metadata.name", "status.capvcd.version" portion of the RDE, specPatch map should be something like this -
// specPatch["CapiYaml"] = updated-yaml
// metadataPatch["Name"] = updated-name
// capvcdStatusPatch["Version"] = updated-version
func (client *Client) PatchRDE(ctx context.Context, specPatch, metadataPatch, capvcdStatusPatch map[string]interface{}, rdeID string) (rde *swagger.DefinedEntity, err error) {
	defer func() {
		// recover from panic if panic occurs because of
		// 1. calling Set() on a zero value
		if r := recover(); r != nil {
			klog.Errorf("panic occurred while patching RDE: [%v]", r)
			err = errors.Errorf("recovered panic during updating entity: [%v]", r)
		}
	}()
	for retries := 0; retries < MaxUpdateRetries; retries++ {
		rde, resp, etag, err := client.ApiClient.DefinedEntityApi.GetDefinedEntity(ctx, rdeID)
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
		rde, resp, err = client.ApiClient.DefinedEntityApi.UpdateDefinedEntity(ctx, rde, etag, rdeID, nil)
		if err != nil {
			klog.Errorf("failed to update defined entity with ID [%s]: [%v]. Remaining retry attempts: [%d]", rdeID, err, MaxUpdateRetries-retries+1)
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
func (client *Client) GetCAPVCDEntity(ctx context.Context, rdeID string) (*swagger.DefinedEntity, *rdeType.CAPVCDSpec, *rdeType.Metadata, *rdeType.CAPVCDStatus, error) {
	rde, resp, _, err := client.ApiClient.DefinedEntityApi.GetDefinedEntity(ctx, rdeID)
	if err != nil {
		return nil, nil, nil, nil, fmt.Errorf("failed to get defined entity with ID [%s]: [%v]", rdeID, err)
	}
	if resp.StatusCode != http.StatusOK {
		return nil, nil, nil, nil, fmt.Errorf("error getting defined entity with ID [%s]: [%v]", rdeID, err)
	}
	capvcdEntity, err := vcdutil.ConvertMapToCAPVCDEntity(rde.Entity)
	if err != nil {
		return nil, nil, nil, nil, fmt.Errorf("failed to convert CAPVCD entity map to type CAPVCD entity: [%v]", err)
	}
	return &rde, &capvcdEntity.Spec, &capvcdEntity.Metadata, &capvcdEntity.Status.CAPVCDStatus, nil
}
