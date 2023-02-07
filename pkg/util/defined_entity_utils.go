package util

import (
	"encoding/json"
	"fmt"
	rdeType "github.com/vmware/cluster-api-provider-cloud-director/pkg/vcdtypes/rde_type_1_2_0"
)

func ConvertMapToCAPVCDEntity(entityMap map[string]interface{}) (*rdeType.CAPVCDEntity, error) {
	var capvcdEntity rdeType.CAPVCDEntity
	entityByteArr, err := json.Marshal(&entityMap)
	if err != nil {
		return nil, fmt.Errorf("failed to unmarshal entity map: [%v]", err)
	}
	err = json.Unmarshal(entityByteArr, &capvcdEntity)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal entity byte array to capvcd entity: [%v]", err)
	}
	return &capvcdEntity, nil
}

func ConvertCAPVCDEntityToMap(capvcdEntity *rdeType.CAPVCDEntity) (map[string]interface{}, error) {
	var entityMap map[string]interface{}
	entityByteArr, err := json.Marshal(&capvcdEntity)
	if err != nil {
		return nil, fmt.Errorf("failed to unmarshal CAPVCD entity to byte array: [%v]", err)
	}
	err = json.Unmarshal(entityByteArr, &entityMap)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal CAPVCD entity data to a map: [%v]", err)
	}
	return entityMap, nil
}

func ConvertMapToCAPVCDStatus(capvcdStatusMap map[string]interface{}) (*rdeType.CAPVCDStatus, error) {
	var capvcdStatus rdeType.CAPVCDStatus
	entityByteArr, err := json.Marshal(&capvcdStatusMap)
	if err != nil {
		return nil, fmt.Errorf("failed to unmarshal entity map: [%v]", err)
	}
	err = json.Unmarshal(entityByteArr, &capvcdStatus)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal entity byte array to capvcd entity: [%v]", err)
	}
	return &capvcdStatus, nil
}
func ConvertCAPVCDStatusToMap(capvcdStatus *rdeType.CAPVCDStatus) (map[string]interface{}, error) {
	var capvcdStatusMap map[string]interface{}
	entityByteArr, err := json.Marshal(&capvcdStatus)
	if err != nil {
		return nil, fmt.Errorf("failed to unmarshal CAPVCD entity to byte array: [%v]", err)
	}
	err = json.Unmarshal(entityByteArr, &capvcdStatusMap)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal CAPVCD entity data to a map: [%v]", err)
	}
	return capvcdStatusMap, nil
}
