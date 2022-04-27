package vcdcapiclient

import (
	"context"
	"fmt"
	"github.com/pkg/errors"
	"github.com/vmware/cloud-provider-for-cloud-director/pkg/vcdclient"
	swagger "github.com/vmware/cloud-provider-for-cloud-director/pkg/vcdswaggerclient"
	"github.com/vmware/cluster-api-provider-cloud-director/pkg/util"
	vcdutil "github.com/vmware/cluster-api-provider-cloud-director/pkg/util"
	"github.com/vmware/cluster-api-provider-cloud-director/pkg/vcdtypes"
	"k8s.io/klog"
	"net/http"
	"reflect"
	"strings"
)

const (
	MaxUpdateRetries = 10
)

type VCDCAPIClient struct {
	VCDClient *vcdclient.Client
}

// SetIsManagementClusterInRDE sets the isManagementCluster flag in RDE for the management cluster
func (client *VCDCAPIClient) SetIsManagementClusterInRDE(ctx context.Context, managementClusterRDEId string) error {

	// TODO: This method is currently not used anywhere in the code. This is just a placeholder function to preserve logic

	if managementClusterRDEId == "" {
		klog.V(3).Infof("RDE ID for the management cluster not found. Skip setting isManagementCluster flag for the RDE.")
		return nil
	}
	updatePatch := make(map[string]interface{})
	updatePatch["Status.IsManagementCluster"] = true
	_, err := client.PatchRDE(ctx, updatePatch, managementClusterRDEId)
	if err != nil {
		return fmt.Errorf("failed to set isManagementCluster flag for management cluster with RDE ID [%s]: [%v]", managementClusterRDEId, err)
	}
	return nil
}

// PatchRDE updates only specific fields in the RDE. Takes in a map with keys, which contain "." delimitted
// strings, representing the CAPVCD RDE fields to be updated.
// Example: To patch only the API version for the RDE
func (client *VCDCAPIClient) PatchRDE(ctx context.Context, patch map[string]interface{}, rdeID string) (rde *swagger.DefinedEntity, err error) {
	defer func() {
		// recover from panic if panic occurs because of
		// 1. calling Set() on a zero value
		if r := recover(); r != nil {
			klog.Errorf("panic occurred while patching RDE: [%v]", r)
			err = errors.Errorf("recovered panic during updating entity: [%v]", r)
		}
	}()
	for retries := 0; retries < MaxUpdateRetries; retries++ {
		rde, resp, etag, err := client.VCDClient.APIClient.DefinedEntityApi.GetDefinedEntity(ctx, rdeID)
		if err != nil {
			return nil, fmt.Errorf("failed to call get defined entity RDE with ID [%s]: [%s]", rdeID, err)
		}
		if len(patch) == 0 {
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

		for k, v := range patch {
			fields := strings.Split(k, ".")
			updatedVal := reflect.ValueOf(v)
			klog.V(4).Infof("Assigning value %v to key %s", v, k)
			objVal := reflect.ValueOf(capvcdEntity).Elem()
			for _, attr := range fields {
				// cannot call fieldByName on a zero value
				objVal = objVal.FieldByName(attr)
				if objVal.Kind() == reflect.Ptr {
					objVal = objVal.Elem()
				}
			}
			objVal.Set(updatedVal)
		}

		// update the defined entity
		capvcdEntityMap, err := util.ConvertCAPVCDEntityToMap(capvcdEntity)
		if err != nil {
			return nil, fmt.Errorf("failed to convert CAPVCD entity to map: [%v]", err)
		}
		rde.Entity = capvcdEntityMap
		rde, resp, err = client.VCDClient.APIClient.DefinedEntityApi.UpdateDefinedEntity(ctx, rde, etag, rdeID, nil)
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

func (client *VCDCAPIClient) GetCAPVCDEntity(ctx context.Context, rdeID string) (*swagger.DefinedEntity, *vcdtypes.CAPVCDEntity, error) {
	rde, resp, _, err := client.VCDClient.APIClient.DefinedEntityApi.GetDefinedEntity(ctx, rdeID)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to get defined entity with ID [%s]: [%v]", rdeID, err)
	}
	if resp.StatusCode != http.StatusOK {
		return nil, nil, fmt.Errorf("error getting defined entity with ID [%s]: [%v]", rdeID, err)
	}
	capvcdEntity, err := vcdutil.ConvertMapToCAPVCDEntity(rde.Entity)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to convert CAPVCD entity map to type CAPVCD entity: [%v]", err)
	}
	return &rde, capvcdEntity, nil
}
