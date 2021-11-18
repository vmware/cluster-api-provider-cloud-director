package vcdclient

import (
	"context"
	"fmt"
	"github.com/vmware/cluster-api-provider-cloud-director/pkg/util"
	"net/http"
	"reflect"
	"strings"
)

func (client *Client) UpdateDefinedEntityWithChanges(ctx context.Context, patch map[string]interface{}, definedEntityID string) error {
	definedEntity, resp, etag, err := client.ApiClient.DefinedEntityApi.GetDefinedEntity(ctx, definedEntityID)
	if err != nil {
		return fmt.Errorf("failed to call get defined entity RDE with ID [%s]: [%s]", definedEntityID, err)
	}

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("error getting the defined entity with ID [%s]", definedEntityID)
	}

	capvcdEntity, err := util.ConvertMapToCAPVCDEntity(definedEntity.Entity)
	if err != nil {
		return fmt.Errorf("failed to convert map to CAPVCD entity [%v]", err)
	}

	for k, v := range patch {
		fields := strings.Split(k, ".")
		updatedVal := reflect.ValueOf(v)
		objVal := reflect.ValueOf(capvcdEntity).Elem()
		for _, attr := range fields {
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
		return fmt.Errorf("failed to convert CAPVCD entity to map: [%v]", err)
	}
	definedEntity.Entity = capvcdEntityMap
	_, resp, err = client.ApiClient.DefinedEntityApi.UpdateDefinedEntity(ctx, definedEntity, etag, definedEntityID, nil)
	if err != nil {
		return fmt.Errorf("failed to update defined entity with ID [%s]: [%v]", definedEntityID, err)
	}
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("error updating the defined entity with ID [%s]. failed with status code [%d]", definedEntityID, resp.StatusCode)
	}
	return nil
}
