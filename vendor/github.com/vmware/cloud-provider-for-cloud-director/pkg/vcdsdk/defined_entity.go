package vcdsdk

import (
	"context"
	"encoding/json"
	"fmt"
	swaggerClient "github.com/vmware/cloud-provider-for-cloud-director/pkg/vcdswaggerclient"
	"k8s.io/klog"
	"net/http"
	"strings"
)

const (
	NoRdePrefix = `NO_RDE_`

	MaxRDEUpdateRetries = 10

	ComponentStatusFieldVCDResourceSet = "vcdResourceSet"

	ComponentCPI    = "cpi"
	ComponentCSI    = "csi"
	ComponentCAPVCD = "capvcd"

	// VCD resource types in VCDResourceSet
	VcdResourceVirtualService   = "virtual-service"
	VcdResourceLoadBalancerPool = "lb-pool"
	VcdResourceDNATRule         = "dnat-rule"
	VcdResourceAppPortProfile   = "app-port-profile"

	CAPVCDEntityTypeVendor = "vmware"
	CAPVCDEntityTypeNss    = "capvcdCluster"

	NativeClusterEntityTypeVendor = "cse"
	NativeClusterEntityTypeNss    = "nativeCluster"
)

type VCDResource struct {
	Type              string                 `json:"type,omitempty"`
	ID                string                 `json:"id,omitempty"`
	Name              string                 `json:"name,omitempty"`
	AdditionalDetails map[string]interface{} `json:"additionalDetails,omitempty"`
}

type ComponentStatus struct {
	VCDResourceSet []VCDResource `json:"vcdResourceSet,omitempty"`
}

type RDEManager struct {
	Client                 *Client
	StatusComponentName    string
	StatusComponentVersion string
	ClusterID              string
}

func NewRDEManager(client *Client, clusterID string, statusComponentName string, statusComponentVersion string) *RDEManager {
	return &RDEManager{
		Client:                 client,
		ClusterID:              clusterID,
		StatusComponentName:    statusComponentName,
		StatusComponentVersion: statusComponentVersion,
	}
}

func IsValidEntityId(entityTypeID string) bool {
	return entityTypeID != "" && !strings.HasPrefix(entityTypeID, NoRdePrefix)
}

func IsCAPVCDEntityType(entityTypeID string) bool {
	entityTypeIDSplit := strings.Split(entityTypeID, ":")
	// format is urn:vcloud:type:<vendor>:<nss>:<version>
	if len(entityTypeIDSplit) != 6 {
		return false
	}
	return entityTypeIDSplit[3] == CAPVCDEntityTypeVendor && entityTypeIDSplit[4] == CAPVCDEntityTypeNss
}

func IsNativeClusterEntityType(entityTypeID string) bool {
	entityTypeIDSplit := strings.Split(entityTypeID, ":")
	// format is urn:vcloud:type:<vendor>:<nss>:<version>
	if len(entityTypeIDSplit) != 6 {
		return false
	}
	return entityTypeIDSplit[3] == NativeClusterEntityTypeVendor && entityTypeIDSplit[4] == NativeClusterEntityTypeNss
}

func convertMapToComponentStatus(componentStatusMap map[string]interface{}) (*ComponentStatus, error) {
	componentStatusBytes, err := json.Marshal(componentStatusMap)
	if err != nil {
		return nil, fmt.Errorf("failed to convert componentStatusMap to byte array: [%v]", err)
	}

	var cs ComponentStatus
	err = json.Unmarshal(componentStatusBytes, &cs)
	if err != nil {
		return nil, fmt.Errorf("failed to read bytes from componentStatus [%#v] to ComponentStatus object: [%v]", componentStatusMap, err)
	}

	return &cs, nil
}

func addToVCDResourceSet(component string, componentName string, componentVersion string, statusMap map[string]interface{}, vcdResource VCDResource) (map[string]interface{}, error) {
	// get the component info from the status
	componentIf, ok := statusMap[component]
	if !ok {
		// component map not found
		statusMap[component] = map[string]interface{}{
			"name":    componentName,
			"version": componentVersion,
			"vcdResourceSet": []VCDResource{
				vcdResource,
			},
		}
		klog.Infof("created component map [%#v] since the component was not found in the status map", statusMap[component])
		return statusMap, nil
	}

	componentMap, ok := componentIf.(map[string]interface{})
	if !ok {
		return nil, fmt.Errorf("failed to convert the status belonging to component [%s] to map[string]interface{}", component)
	}

	componentStatus, err := convertMapToComponentStatus(componentMap)
	if err != nil {
		return nil, fmt.Errorf("failed to convert component status map to ")
	}

	if componentStatus.VCDResourceSet == nil || len(componentStatus.VCDResourceSet) == 0 {
		// create an array with a single element - vcdResource
		componentMap[ComponentStatusFieldVCDResourceSet] = []VCDResource{
			vcdResource,
		}
		return statusMap, nil
	}

	resourceFound := false
	// check if vcdResource is already present
	for idx, resource := range componentStatus.VCDResourceSet {
		if resource.ID == vcdResource.ID && resource.Type == vcdResource.Type {
			resourceFound = true
			componentStatus.VCDResourceSet[idx].AdditionalDetails = vcdResource.AdditionalDetails
		}
	}

	if !resourceFound {
		componentStatus.VCDResourceSet = append(componentStatus.VCDResourceSet, vcdResource)
	}

	componentMap[ComponentStatusFieldVCDResourceSet] = componentStatus.VCDResourceSet
	return statusMap, nil
}

// AddToVCDResourceSet adds a VCDResource to the VCDResourceSet of the component in the RDE
func (rdeManager *RDEManager) AddToVCDResourceSet(ctx context.Context, component string, resourceType string,
	resourceName string, resourceId string, additionalDetails map[string]interface{}) error {
	if rdeManager.ClusterID == "" || strings.HasPrefix(rdeManager.ClusterID, NoRdePrefix) {
		// Indicates that the RDE ID is either empty or it was auto-generated.
		klog.Infof("ClusterID [%s] is empty or generated, hence not adding VCDResource [%s:%s] from RDE",
			rdeManager.ClusterID, resourceType, resourceId)
		return nil
	}
	for i := MaxRDEUpdateRetries; i > 1; i-- {
		rde, resp, etag, err := rdeManager.Client.APIClient.DefinedEntityApi.GetDefinedEntity(ctx, rdeManager.ClusterID)
		if resp != nil && resp.StatusCode != http.StatusOK {
			var responseMessageBytes []byte
			if gsErr, ok := err.(swaggerClient.GenericSwaggerError); ok {
				responseMessageBytes = gsErr.Body()
			}
			return fmt.Errorf(
				"failed to get RDE [%s] when adding resourse to VCD resource set ; expected http response [%v], obtained [%v]: resp: [%#v]: [%v]",
				rdeManager.ClusterID, http.StatusOK, resp.StatusCode, string(responseMessageBytes), err)
		} else if err != nil {
			return fmt.Errorf("error while updating the RDE [%s]: [%v]", rdeManager.ClusterID, err)
		}

		// Only entity of type capvcdCluster should be updated.
		if !IsCAPVCDEntityType(rde.EntityType) {
			nonCapvcdEntityError := NonCAPVCDEntityError{
				EntityTypeID: rde.EntityType,
			}
			return nonCapvcdEntityError
		}
		statusIf, ok := rde.Entity["status"]
		if !ok {
			return fmt.Errorf("failed to update RDE [%s] with VCDResourse set information", rdeManager.ClusterID)
		}
		statusMap, ok := statusIf.(map[string]interface{})
		if !ok {
			return fmt.Errorf("failed to convert RDE status [%s] to map[string]interface{}", rdeManager.ClusterID)
		}
		vcdResource := VCDResource{
			Type:              resourceType,
			ID:                resourceId,
			Name:              resourceName,
			AdditionalDetails: additionalDetails,
		}
		updatedStatusMap, err := addToVCDResourceSet(component, rdeManager.StatusComponentName,
			rdeManager.StatusComponentVersion, statusMap, vcdResource)
		if err != nil {
			return fmt.Errorf("error occurred when updating VCDResource set of %s status in RDE [%s]: [%v]", rdeManager.ClusterID, component, err)
		}
		rde.Entity["status"] = updatedStatusMap
		_, resp, err = rdeManager.Client.APIClient.DefinedEntityApi.UpdateDefinedEntity(ctx, rde, etag, rdeManager.ClusterID, nil)
		if resp != nil {
			if resp.StatusCode == http.StatusPreconditionFailed {
				klog.Errorf("wrong etag while adding [%v] to VCDResourceSet in RDE [%s]. Retry attempts remaining: [%d]", vcdResource, rdeManager.ClusterID, i-1)
				continue
			} else if resp.StatusCode != http.StatusOK {
				var responseMessageBytes []byte
				if gsErr, ok := err.(swaggerClient.GenericSwaggerError); ok {
					responseMessageBytes = gsErr.Body()
				}
				return fmt.Errorf(
					"failed to add resource [%s] having ID [%s] to VCDResourseSet of %s in RDE [%s]; expected http response [%v], obtained [%v]: resp: [%#v]: [%v]",
					vcdResource.Name, vcdResource.ID, component, rdeManager.ClusterID, http.StatusOK, resp.StatusCode, string(responseMessageBytes), err)
			}
			// resp.StatusCode is http.StatusOK
			klog.Infof("successfully added resource [%s] having ID [%s] to VCDResourceSet of [%s] in RDE [%s]",
				vcdResource.Name, vcdResource.ID, component, rdeManager.ClusterID)
			return nil
		} else if err != nil {
			return fmt.Errorf("error while updating the RDE [%s]: [%v]", rdeManager.ClusterID, err)
		} else {
			return fmt.Errorf("invalid response obtained when updating VCDResoruceSet of %s in RDE [%s]", component, rdeManager.ClusterID)
		}
	}
	return nil
}

// removeFromVCDResourceSet removes a VCDResource from VCDResourceSet if type and name of the resource matches
// If the component is absent, an empty component is initialized.
func removeFromVCDResourceSet(component string, componentName string, componentVersion string,
	statusMap map[string]interface{}, vcdResource VCDResource) (map[string]interface{}, error) {
	// get the component info from the status
	componentIf, ok := statusMap[component]
	if !ok {
		// component map not found. Recreate with empty values
		statusMap[component] = map[string]interface{}{
			"name":           componentName,
			"version":        componentVersion,
			"vcdResourceSet": []VCDResource{},
		}
		klog.Infof("created component map [%#v] since the component was not found in the status map", statusMap[component])
		return statusMap, nil
	}

	componentMap, ok := componentIf.(map[string]interface{})
	if !ok {
		return nil, fmt.Errorf("failed to convert the status belonging to component [%s] to map[string]interface{}", component)
	}

	componentStatus, err := convertMapToComponentStatus(componentMap)
	if err != nil {
		return nil, fmt.Errorf("failed to convert component status map to ")
	}

	resourceFound := false
	updatedVcdResourceSet := make([]VCDResource, 0)
	for _, resource := range componentStatus.VCDResourceSet {
		if resource.Name == vcdResource.Name && resource.Type == vcdResource.Type {
			// remove the resource from the array
			resourceFound = true
			continue
		}
		updatedVcdResourceSet = append(updatedVcdResourceSet, resource)
	}

	if !resourceFound {
		klog.Infof("VCDResource having name [%s] and Type [%s] was not found in the component [%s]", vcdResource.Name, vcdResource.Type, component)
		return statusMap, nil
	}

	componentMap[ComponentStatusFieldVCDResourceSet] = updatedVcdResourceSet
	return statusMap, nil
}

// RemoveFromVCDResourceSet removes a VCD resource from the VCDResourceSet property belonging to a component (CPI, CSI or CAPVCD)
// Removal of a VCDResource from VCDResourceSet is done by name to support removal of the resource from RDE on retries if the resource
// has been deleted from VCD
func (rdeManager *RDEManager) RemoveFromVCDResourceSet(ctx context.Context, component, resourceType, resourceName string) error {
	if rdeManager.ClusterID == "" || strings.HasPrefix(rdeManager.ClusterID, NoRdePrefix) {
		klog.Infof("ClusterID [%s] is empty or generated, hence not removing VCDResource [%s:%s] from RDE",
			rdeManager.ClusterID, resourceType, resourceName)
		return nil
	}
	for i := MaxRDEUpdateRetries; i > 1; i-- {
		rde, resp, etag, err := rdeManager.Client.APIClient.DefinedEntityApi.GetDefinedEntity(ctx, rdeManager.ClusterID)
		if resp != nil && resp.StatusCode != http.StatusOK {
			var responseMessageBytes []byte
			if gsErr, ok := err.(swaggerClient.GenericSwaggerError); ok {
				responseMessageBytes = gsErr.Body()
			}
			return fmt.Errorf(
				"failed to get RDE with id [%s]; expected http response [%v], obtained [%v]: resp: [%#v]: [%v]",
				rdeManager.ClusterID, http.StatusOK, resp.StatusCode, string(responseMessageBytes), err)
		} else if err != nil {
			return fmt.Errorf("error while updating the RDE [%s]: [%v]", rdeManager.ClusterID, err)
		}

		if !IsCAPVCDEntityType(rde.EntityType) {
			nonCapvcdEntityError := NonCAPVCDEntityError{
				EntityTypeID: rde.EntityType,
			}
			return nonCapvcdEntityError
		}

		statusEntity, ok := rde.Entity["status"]
		if !ok {
			return fmt.Errorf("failed to parse status in RDE [%s] to remove [%s] from %s status", rdeManager.ClusterID, resourceName, component)
		}
		statusMap, ok := statusEntity.(map[string]interface{})
		if !ok {
			return fmt.Errorf("failed to parse status in RDE [%s] into map[string]interface{} to remove [%s] from %s status", rdeManager.ClusterID, resourceName, component)
		}
		updatedStatus, err := removeFromVCDResourceSet(component, rdeManager.StatusComponentName,
			rdeManager.StatusComponentVersion, statusMap, VCDResource{
				Type: resourceType,
				Name: resourceName,
			})
		if err != nil {
			return fmt.Errorf("failed to remove resource [%s] from VCDResourceSet in %s status section of RDE [%s]: [%v]", resourceName, component, rdeManager.ClusterID, err)
		}
		rde.Entity["status"] = updatedStatus

		_, resp, err = rdeManager.Client.APIClient.DefinedEntityApi.UpdateDefinedEntity(ctx, rde, etag, rdeManager.ClusterID, nil)
		if resp != nil {
			if resp.StatusCode == http.StatusPreconditionFailed {
				klog.Errorf("wrong etag while removing [%s] from VCDResourceSet in RDE [%s]. Retry attempts remaining: [%d]", resourceName, rdeManager.ClusterID, i-1)
				continue
			} else if resp.StatusCode != http.StatusOK {
				var responseMessageBytes []byte
				if gsErr, ok := err.(swaggerClient.GenericSwaggerError); ok {
					responseMessageBytes = gsErr.Body()
				}
				return fmt.Errorf(
					"failed to update %s status for RDE [%s]; expected http response [%v], obtained [%v]: resp: [%#v]: [%v]", component,
					rdeManager.ClusterID, http.StatusOK, resp.StatusCode, string(responseMessageBytes), err)
			}
			// resp.StatusCode is http.StatusOK
			klog.Infof("successfully removed resource [%s] of type [%s] from VCDResourceSet of [%s] in RDE [%s]",
				resourceName, resourceType, component, rdeManager.ClusterID)
			return nil
		} else if err != nil {
			return fmt.Errorf("error while removing virtual service [%s] from the RDE [%s]: [%v]", resourceName, rdeManager.ClusterID, err)
		} else {
			return fmt.Errorf("invalid response obtained when updating VCDResoruceSet of %s in RDE [%s]", component, rdeManager.ClusterID)
		}
	}
	return nil
}
