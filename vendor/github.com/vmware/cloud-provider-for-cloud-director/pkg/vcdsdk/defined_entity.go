package vcdsdk

import (
	"context"
	"encoding/json"
	"fmt"
	swaggerClient "github.com/vmware/cloud-provider-for-cloud-director/pkg/vcdswaggerclient"
	"k8s.io/klog"
	"net/http"
	"net/url"
	"strings"
	"time"
)

const (
	NoRdePrefix = `NO_RDE_`

	MaxRDEUpdateRetries                = 10
	DefaultRollingWindowSize           = 10
	ComponentStatusFieldVCDResourceSet = "vcdResourceSet"
	ComponentStatusFieldErrorSet       = "errorSet"
	ComponentStatusFieldEventSet       = "eventSet"

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
	CAPVCDEntityTypePrefix = "urn:vcloud:type:vmware:capvcdCluster"

	NativeClusterEntityTypeVendor = "cse"
	NativeClusterEntityTypeNss    = "nativeCluster"
)

type VCDResource struct {
	Type              string                 `json:"type,omitempty"`
	ID                string                 `json:"id,omitempty"`
	Name              string                 `json:"name,omitempty"`
	AdditionalDetails map[string]interface{} `json:"additionalDetails,omitempty"`
}

// TODO: In the future, add subErrorType to AdditionalDetails to handle/match deeper level components removal such as (virtualServiceError, dnatRuleUpdateError, etc)
// We would need to make AdditionalDetails it's own struct, include a SubErrorType struct/string inside of it
// During removal, we could match certain subErrorTypes such as delete all LB creation errors that were from subErrorType: virtualServiceFailures
// AdditionalDetails structure would look something like:
// additionalDetails : { subErrorType: DNATRuleFailed, additionalInfo: map[string]interface{} }
type BackendError struct {
	Name              string                 `json:"name,omitempty"`
	OccurredAt        time.Time              `json:"occurredAt,omitempty"`
	VcdResourceId     string                 `json:"vcdResourceId,omitempty"`
	VcdResourceName   string                 `json:"vcdResourceName,omitempty"`
	AdditionalDetails map[string]interface{} `json:"additionalDetails,omitempty"`
}

type BackendEvent struct {
	Name              string                 `json:"name,omitempty"`
	OccurredAt        time.Time              `json:"occurredAt,omitempty"`
	VcdResourceId     string                 `json:"vcdResourceId,omitempty"`
	VcdResourceName   string                 `json:"vcdResourceName,omitempty"`
	AdditionalDetails map[string]interface{} `json:"additionalDetails,omitempty"`
}

type ComponentStatus struct {
	VCDResourceSet []VCDResource  `json:"vcdResourceSet,omitempty"`
	ErrorSet       []BackendError `json:"errorSet,omitempty"`
	EventSet       []BackendEvent `json:"eventSet,omitempty"`
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

// EntityType contains only the required properties in get entity type response
type EntityType struct {
	ID      string                 `json:"id"`
	Name    string                 `json:"name"`
	Nss     string                 `json:"nss"`
	Version string                 `json:"version"`
	Schema  map[string]interface{} `json:"schema"`
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

// AddVCDResourceToStatusMap updates the input status map with VCDResource created from the input parameters. This function doesn't make any
// 	calls to VCD.
func AddVCDResourceToStatusMap(component string, componentName string, componentVersion string, statusMap map[string]interface{}, vcdResource VCDResource) (map[string]interface{}, error) {
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
		return nil, fmt.Errorf("failed to convert component status map to component status object")
	}

	if componentStatus.VCDResourceSet == nil || len(componentStatus.VCDResourceSet) == 0 {
		// create an array with a single element - vcdResource
		componentMap[ComponentStatusFieldVCDResourceSet] = []VCDResource{
			vcdResource,
		}
		componentMap["name"] = componentName
		componentMap["version"] = componentVersion
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

/*
AddToErrorSet function takes BackendError as an input and adds it to the "errorSet" of the specified "componentSectionName" in the RDE status.
It caps the size of the "errorSet" in the specified "componentSectionName" to the "rollingWindowSize", by removing the oldest entries.

It raises errors on below conditions. It is caller/component's responsibility to distinguish the errors as either hard (or) soft failures.
 - If rdeId is not valid or empty.
 - If rde.entity.status section is missing
 - If rde is not of type capvcdCluster
 - On any failures while updating the RDE.

Below is the sample structure of the RDE this function operates on. <componentSectionName> could be "csi", "cpi", "capvcd", "vkp"
status:
  <componentSectionName>:
     errorSet:
       - <newError>
*/

// TODO requested from Sahithi:
// 1. Implement a method that takes list of errors (or) list of events
// 2. When 1 is implemented, modify removeErrorByNameOrIdFromErrorSet to sort elements by timestamps before removal
// as the order coming in are not necessary sorted by timestamps
// 3.A method that will remove errors and addition of success event at the same time
func (rdeManager *RDEManager) AddToErrorSet(ctx context.Context, componentSectionName string, newError BackendError, rollingWindowSize int) error {
	if rdeManager.ClusterID == "" || strings.HasPrefix(rdeManager.ClusterID, NoRdePrefix) {
		// Indicates that the RDE ID is either empty or it was auto-generated.
		klog.Infof("ClusterID [%s] is empty or generated, hence cannot add errors [%v] to RDE",
			rdeManager.ClusterID, newError)
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
				"failed to get RDE [%s] when adding error to the errorSet of [%s] ; expected http response [%v], obtained [%v]: resp: [%#v]: [%v]",
				rdeManager.ClusterID, componentSectionName, http.StatusOK, resp.StatusCode, string(responseMessageBytes), err)
		} else if err != nil {
			return fmt.Errorf("error retrieving the RDE [%s]: [%v], while adding to errorSet", rdeManager.ClusterID, err)
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
			return fmt.Errorf("missing RDE status section in [%s], while adding error [%v] to the errorSet", rdeManager.ClusterID, newError)
		}
		statusMap, ok := statusIf.(map[string]interface{})
		if !ok {
			return fmt.Errorf("failed to convert RDE status [%s] to map[string]interface{}, while adding error [%s] to the errorSet", rdeManager.ClusterID, newError)
		}

		// update the statusMap (in memory) with the new Error in the specified componentSection
		updatedStatusMap, err := rdeManager.updateComponentMapWithNewError(componentSectionName, statusMap, newError, rollingWindowSize)
		if err != nil {
			return fmt.Errorf("error occurred when updating error set of [%s] status in RDE [%s]: [%v]", componentSectionName, rdeManager.ClusterID, err)
		}
		rde.Entity["status"] = updatedStatusMap

		// persist the updated statusMap to VCD
		_, resp, err = rdeManager.Client.APIClient.DefinedEntityApi.UpdateDefinedEntity(ctx, rde, etag, rdeManager.ClusterID, nil)
		if resp != nil {
			if resp.StatusCode == http.StatusPreconditionFailed {
				klog.Errorf("wrong etag while adding newError [%v] in RDE [%s]. Retry attempts remaining: [%d]", newError, rdeManager.ClusterID, i-1)
				continue
			} else if resp.StatusCode != http.StatusOK {
				var responseMessageBytes []byte
				if gsErr, ok := err.(swaggerClient.GenericSwaggerError); ok {
					responseMessageBytes = gsErr.Body()
				}
				return fmt.Errorf(
					"failed to add newError [%v] in componentSectionName [%s] of RDE [%s]; expected http response [%v], obtained [%v]: resp: [%#v]: [%v]",
					newError, componentSectionName, rdeManager.ClusterID, http.StatusOK, resp.StatusCode, string(responseMessageBytes), err)
			}
			// resp.StatusCode is http.StatusOK
			klog.Infof("successfully added newError [%v] in componentSectionName [%s] of RDE [%s]",
				newError, componentSectionName, rdeManager.ClusterID)
			return nil
		} else if err != nil {
			return fmt.Errorf("error while updating the RDE [%s]: [%v]", rdeManager.ClusterID, err)
		} else {
			return fmt.Errorf("invalid response obtained when updating newError [%v] of componentSectionName [%s] in RDE [%s]", newError, componentSectionName, rdeManager.ClusterID)
		}
	}

	return nil
}

/*
RemoveErrorByNameOrIdFromErrorSet Given a name (and/or) vcdResourceId of the error, it removes all the matching entries from the errorset of the
specified component section in the RDE.status.

Note that vcdResourceId parameter is optional. If a non-empty vcdResourceId is passed, it tries to match both the errorName and vcdResourceId

Below is the RDE portion this function operates on
RDE.entity
 status
   <componentSectionName> //capvcd, csi, cpi, vkp
       errorSet
          <controlPlaneError>
          <cloudInitError>
*/
func (rdeManager *RDEManager) RemoveErrorByNameOrIdFromErrorSet(ctx context.Context, componentSectionName string, errorName string, vcdResourceId string, vcdResourceName string) error {
	if rdeManager.ClusterID == "" || strings.HasPrefix(rdeManager.ClusterID, NoRdePrefix) {
		// Indicates that the RDE ID is either empty or it was auto-generated.
		klog.Infof("ClusterID [%s] is empty or generated, hence cannot remove any errors of name [%s] from RDE",
			rdeManager.ClusterID, errorName)
		return nil
	}
	if errorName == "" {
		return fmt.Errorf("errorName cannot be empty, while removing error from the errorSet of [%s]", rdeManager.ClusterID)
	}
	for i := MaxRDEUpdateRetries; i > 1; i-- {
		rde, resp, etag, err := rdeManager.Client.APIClient.DefinedEntityApi.GetDefinedEntity(ctx, rdeManager.ClusterID)
		if resp != nil && resp.StatusCode != http.StatusOK {
			var responseMessageBytes []byte
			if gsErr, ok := err.(swaggerClient.GenericSwaggerError); ok {
				responseMessageBytes = gsErr.Body()
			}
			return fmt.Errorf(
				"failed to get RDE [%s] when removing error from the errorSet of [%s] ; expected http response [%v], obtained [%v]: resp: [%#v]: [%v]",
				rdeManager.ClusterID, componentSectionName, http.StatusOK, resp.StatusCode, string(responseMessageBytes), err)
		} else if err != nil {
			return fmt.Errorf("error retrieving the RDE [%s]: [%v], while removing error [%s] from errorSet", rdeManager.ClusterID, err, errorName)
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
			return fmt.Errorf("missing RDE status section in [%s], while removing error [%s] from the errorSet", rdeManager.ClusterID, errorName)
		}
		statusMap, ok := statusIf.(map[string]interface{})
		if !ok {
			return fmt.Errorf("failed to convert RDE status [%s] to map[string]interface{}, while removing error [%s] from the errorSet", rdeManager.ClusterID, errorName)
		}

		// update the statusMap (in memory) with the new Error in the specified componentSection
		updatedStatusMap, matchingErrorsRemoved, err := rdeManager.removeErrorsfromComponentMap(componentSectionName, statusMap, errorName, vcdResourceId, vcdResourceName)
		if err != nil {
			return fmt.Errorf("error occurred when removing error [%s] from the error set of [%s] status in RDE [%s]: [%v]", errorName, componentSectionName, rdeManager.ClusterID, err)
		}
		if !matchingErrorsRemoved {
			klog.V(3).Infof("No matching errors found and removed from the existing error set of [%s] of RDE [%s]", componentSectionName, rdeManager.ClusterID)
			return nil
		}
		rde.Entity["status"] = updatedStatusMap

		// persist the updated statusMap to VCD
		_, resp, err = rdeManager.Client.APIClient.DefinedEntityApi.UpdateDefinedEntity(ctx, rde, etag, rdeManager.ClusterID, nil)
		if resp != nil {
			if resp.StatusCode == http.StatusPreconditionFailed {
				klog.Errorf("wrong etag while removing error(s) of name [%s] in RDE [%s]. Retry attempts remaining: [%d]", errorName, rdeManager.ClusterID, i-1)
				continue
			} else if resp.StatusCode != http.StatusOK {
				var responseMessageBytes []byte
				if gsErr, ok := err.(swaggerClient.GenericSwaggerError); ok {
					responseMessageBytes = gsErr.Body()
				}
				return fmt.Errorf(
					"failed to remove errors [%s] in componentSectionName [%s] of RDE [%s]; expected http response [%v], obtained [%v]: resp: [%#v]: [%v]",
					errorName, componentSectionName, rdeManager.ClusterID, http.StatusOK, resp.StatusCode, string(responseMessageBytes), err)
			}
			// resp.StatusCode is http.StatusOK
			klog.Infof("successfully removed errors of type [%s] in componentSectionName [%s] of RDE [%s]",
				errorName, componentSectionName, rdeManager.ClusterID)
			return nil
		} else if err != nil {
			return fmt.Errorf("error while updating the RDE [%s]: [%v]", rdeManager.ClusterID, err)
		} else {
			return fmt.Errorf("invalid response obtained when removing errors of type [%s] of componentSectionName [%s] in RDE [%s]", errorName, componentSectionName, rdeManager.ClusterID)
		}
	}

	return nil
}

/*
function updateComponentMapWithNewEvent updates the local (in memory) rde status map with the specified new event.
It also ensures the length of the "eventSet" is capped at the specified "rollingWindowSize" by removing the old entries.

This function does NOT persist the data into VCD.
*/
func (rdeManager *RDEManager) updateComponentMapWithNewEvent(componentName string, statusMap map[string]interface{}, newEvent BackendEvent, rollingWindowSize int) (map[string]interface{}, error) {
	// get the component info from the status
	componentIf, ok := statusMap[componentName]
	if !ok {
		// component map not found
		statusMap[componentName] = map[string]interface{}{
			"name":     rdeManager.StatusComponentName,
			"version":  rdeManager.StatusComponentVersion,
			"eventSet": []BackendEvent{newEvent},
		}
		klog.Infof("created component map [%#v] since the component was not found in the status map", statusMap[componentName])
		return statusMap, nil
	}
	componentMap, ok := componentIf.(map[string]interface{})
	if !ok {
		return nil, fmt.Errorf("failed to convert the status belonging to component [%s] to map[string]interface{}", componentName)
	}
	componentStatus, err := convertMapToComponentStatus(componentMap)
	if err != nil {
		return nil, fmt.Errorf("failed to convert component status [%s] map to Component object", componentName)
	}

	newSize := len(componentStatus.EventSet) + 1
	componentStatus.EventSet = append(componentStatus.EventSet, newEvent)
	if newSize > rollingWindowSize {
		componentStatus.EventSet = componentStatus.EventSet[1:]
	}
	componentMap[ComponentStatusFieldEventSet] = componentStatus.EventSet
	return statusMap, nil
}

/*
function updateComponentMapWithNewError updates the local (in memory) rde status map with the specified new error.
It also ensures the length of the "errorSet" is capped at the specified "rollingWindowSize" by removing the old entries.
This function does NOT persist the data into VCD.
*/
func (rdeManager *RDEManager) updateComponentMapWithNewError(componentRdeSectionName string, statusMap map[string]interface{}, newError BackendError, rollingWindowSize int) (map[string]interface{}, error) {
	// get the component info from the status
	componentIf, ok := statusMap[componentRdeSectionName]
	if !ok {
		// component map not found
		statusMap[componentRdeSectionName] = map[string]interface{}{
			"name":     rdeManager.StatusComponentName,
			"version":  rdeManager.StatusComponentVersion,
			"errorSet": []BackendError{newError},
		}
		klog.Infof("created component map [%#v] since the component was not found in the status map", statusMap[componentRdeSectionName])
		return statusMap, nil
	}
	componentMap, ok := componentIf.(map[string]interface{})
	if !ok {
		return nil, fmt.Errorf("failed to convert the status belonging to component [%s] to map[string]interface{}", componentRdeSectionName)
	}
	componentStatus, err := convertMapToComponentStatus(componentMap)
	if err != nil {
		return nil, fmt.Errorf("failed to convert component status map [%s] to Component object", componentRdeSectionName)
	}

	newSize := len(componentStatus.ErrorSet) + 1
	componentStatus.ErrorSet = append(componentStatus.ErrorSet, newError)
	if newSize > rollingWindowSize {
		componentStatus.ErrorSet = componentStatus.ErrorSet[1:]
	}
	componentMap[ComponentStatusFieldErrorSet] = componentStatus.ErrorSet
	return statusMap, nil
}

/*
AddToEventSet function takes BackendEvent as an input and adds it to the "eventSet" of the specified "componentSectionName" in the RDE status.
It caps the size of the "eventSet" in the specified "componentSectionName" to the "rollingWindowSize" by removing the oldest entries.

It raises errors on below conditions. It is caller/component's responsibility to distinguish the errors as either hard (or) soft failures.
 - If rdeId is not valid or empty.
 - If rde.entity.status section is missing
 - If rde is not of type capvcdCluster
 - On any failures while updating the RDE.

Below is the sample structure of the RDE this function operates on.
status:
  <componentSectionName>:
     eventSet:
       - <newEvent>
       - existingEvent
*/
func (rdeManager *RDEManager) AddToEventSet(ctx context.Context, componentSectionName string, newEvent BackendEvent, rollingWindowSize int) error {
	if rdeManager.ClusterID == "" || strings.HasPrefix(rdeManager.ClusterID, NoRdePrefix) {
		// Indicates that the RDE ID is either empty or it was auto-generated.
		klog.Infof("ClusterID [%s] is empty or generated, hence cannot add events [%#v] to RDE",
			rdeManager.ClusterID, newEvent)
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
				"failed to get RDE [%s] when adding event to the eventSet of [%s] ; expected http response [%v], obtained [%v]: resp: [%#v]: [%v]",
				rdeManager.ClusterID, componentSectionName, http.StatusOK, resp.StatusCode, string(responseMessageBytes), err)
		} else if err != nil {
			return fmt.Errorf("error retrieving the RDE [%s]: [%v], while adding to eventSet", rdeManager.ClusterID, err)
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
			return fmt.Errorf("missing RDE status section in [%s], while adding event [%s] to the eventSet", rdeManager.ClusterID, newEvent)
		}
		statusMap, ok := statusIf.(map[string]interface{})
		if !ok {
			return fmt.Errorf("failed to convert RDE status [%s] to map[string]interface{}, while adding event [%#v] to the eventSet ", rdeManager.ClusterID, newEvent)
		}

		// update the statusMap (in memory) with the new Event in the specified componentSection
		updatedStatusMap, err := rdeManager.updateComponentMapWithNewEvent(componentSectionName, statusMap, newEvent, rollingWindowSize)
		if err != nil {
			return fmt.Errorf("error occurred while updating event set of [%s] in RDE [%s]: [%v]", componentSectionName, rdeManager.ClusterID, err)
		}
		rde.Entity["status"] = updatedStatusMap

		// persist the updated statusMap to VCD
		_, resp, err = rdeManager.Client.APIClient.DefinedEntityApi.UpdateDefinedEntity(ctx, rde, etag, rdeManager.ClusterID, nil)
		if resp != nil {
			if resp.StatusCode == http.StatusPreconditionFailed {
				klog.Errorf("wrong etag while adding newEvent [%#v] in RDE [%s]. Retry attempts remaining: [%d]", newEvent, rdeManager.ClusterID, i-1)
				continue
			} else if resp.StatusCode != http.StatusOK {
				var responseMessageBytes []byte
				if gsErr, ok := err.(swaggerClient.GenericSwaggerError); ok {
					responseMessageBytes = gsErr.Body()
				}
				return fmt.Errorf(
					"failed to add newEvent [%#v] in componentSectionName [%s] of RDE [%s]; expected http response [%v], obtained [%v]: resp: [%#v]: [%v]",
					newEvent, componentSectionName, rdeManager.ClusterID, http.StatusOK, resp.StatusCode, string(responseMessageBytes), err)
			}
			// resp.StatusCode is http.StatusOK
			klog.Infof("successfully added newEvent [%v] in componentSectionName [%s] of RDE [%s]",
				newEvent, componentSectionName, rdeManager.ClusterID)
			return nil
		} else if err != nil {
			return fmt.Errorf("error while updating the RDE [%s]: [%v]", rdeManager.ClusterID, err)
		} else {
			return fmt.Errorf("invalid response obtained when updating newEvent [%#v] of componentSectionName [%s] in RDE [%s]", newEvent, componentSectionName, rdeManager.ClusterID)
		}
	}

	return nil
}

func (rdeManager *RDEManager) IsCapvcdEntityTypeRegistered(version string) bool {
	entityTypeID := strings.Join(
		[]string{
			CAPVCDEntityTypePrefix,
			version,
		},
		":",
	)
	entityTypeListUrl, err := rdeManager.Client.VCDClient.Client.OpenApiBuildEndpoint(
		"1.0.0/entityTypes/" + entityTypeID)
	if err != nil {
		klog.Errorf("failed to construct URL to get list of entity types")
		return false
	}
	var output EntityType
	err = rdeManager.Client.VCDClient.Client.OpenApiGetItem(
		rdeManager.Client.VCDClient.Client.APIVersion, entityTypeListUrl,
		url.Values{}, &output, nil)
	if err != nil {
		klog.Errorf("CAPVCD entity type [%s] not registered: [%v]", entityTypeID, err)
		return false
	}
	klog.V(4).Info("Found CAPVCD entity type")
	return true
}

// AddToVCDResourceSet adds a VCDResource to the VCDResourceSet of the component in the RDE
func (rdeManager *RDEManager) AddToVCDResourceSet(ctx context.Context, component string, resourceType string,
	resourceName string, resourceId string, additionalDetails map[string]interface{}) error {
	if rdeManager.ClusterID == "" || strings.HasPrefix(rdeManager.ClusterID, NoRdePrefix) {
		// Indicates that the RDE ID is either empty or it was auto-generated.
		klog.Infof("ClusterID [%s] is empty or generated, hence not adding VCDResource [%s:%s] to RDE",
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
				"failed to get RDE [%s] when adding resource to VCD resource set ; expected http response [%v], obtained [%v]: resp: [%#v]: [%v]",
				rdeManager.ClusterID, http.StatusOK, resp.StatusCode, string(responseMessageBytes), err)
		} else if err != nil {
			return fmt.Errorf("error retrieving the RDE [%s]: [%v], while adding to vcdResourceSet", rdeManager.ClusterID, err)
		}

		// Only entity of type capvcdCluster should be updated.
		if !IsCAPVCDEntityType(rde.EntityType) {
			klog.V(3).Infof("entity type of RDE [%s] is [%s]. skipping adding resource [%s] of type [%s] to status of component [%s]",
				rde.Id, rde.EntityType, resourceName, resourceType, component)
			return nil
		}
		statusIf, ok := rde.Entity["status"]
		if !ok {
			return fmt.Errorf("failed to update RDE [%s] with VCDResource set information", rdeManager.ClusterID)
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
		updatedStatusMap, err := AddVCDResourceToStatusMap(component, rdeManager.StatusComponentName,
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
			klog.Infof("successfully added resource [%s] of type [%s] having ID [%s] to VCDResourceSet of [%s] in RDE [%s]",
				vcdResource.Name, vcdResource.Type, vcdResource.ID, component, rdeManager.ClusterID)
			return nil
		} else if err != nil {
			return fmt.Errorf("error while updating the RDE [%s]: [%v]", rdeManager.ClusterID, err)
		} else {
			return fmt.Errorf("invalid response obtained when updating VCDResoruceSet of %s in RDE [%s]", component, rdeManager.ClusterID)
		}
	}
	return nil
}

// RemoveVCDResourceSetFromStatusMap removes a VCDResource from VCDResourceSet if type and name of the resource matches
// If the component is absent, an empty component is initialized.
func RemoveVCDResourceSetFromStatusMap(component string, componentName string, componentVersion string,
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
		updatedStatus, err := RemoveVCDResourceSetFromStatusMap(component, rdeManager.StatusComponentName,
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

/*
function removeErrorsfromComponentMap updates the local (in memory) rde status map with the specified error(s) removed.
This function does NOT persist the data into VCD.
*/
func (rdeManager *RDEManager) removeErrorsfromComponentMap(componentRdeSectionName string, statusMap map[string]interface{}, errorName string, vcdResourceId string, vcdResourceName string) (map[string]interface{}, bool, error) {
	// get the component info from the status
	componentIf, ok := statusMap[componentRdeSectionName]
	if !ok {
		klog.Infof("missing component [%s] from the rde status, hence skipping removing errors", componentRdeSectionName)
		return nil, false, nil
	}
	componentMap, ok := componentIf.(map[string]interface{})
	if !ok {
		return nil, false, fmt.Errorf("failed to convert the status belonging to component [%s] to map[string]interface{}, while removing error [%s]", componentRdeSectionName, errorName)
	}
	componentStatus, err := convertMapToComponentStatus(componentMap)
	if err != nil {
		return nil, false, fmt.Errorf("failed to convert component status map to Component object")
	}

	// remove errors from the errorSet Map
	// if vcdResourceId is empty, remove all the errors matching the errorName, else remove the errors matching both the
	// errorName and vcdResourceId
	matchingErrorsRemoved := false
	for i := 0; i < len(componentStatus.ErrorSet); i++ {
		// If both vcdResourceId and vcdResourceName are empty, just match the entry with errorName
		// If vcdResourceId is present, it takes the precedence, else match the entry against vcdResourceName.
		if (vcdResourceId == "" && vcdResourceName == "" && componentStatus.ErrorSet[i].Name == errorName) ||
			(vcdResourceId != "" && componentStatus.ErrorSet[i].Name == errorName && componentStatus.ErrorSet[i].VcdResourceId == vcdResourceId) ||
			(vcdResourceId == "" && componentStatus.ErrorSet[i].Name == errorName && vcdResourceName != "" && componentStatus.ErrorSet[i].VcdResourceName == vcdResourceName) {
			componentStatus.ErrorSet = append(componentStatus.ErrorSet[:i], componentStatus.ErrorSet[i+1:]...)
			i--
			matchingErrorsRemoved = true
		}
	}
	componentMap[ComponentStatusFieldErrorSet] = componentStatus.ErrorSet
	return statusMap, matchingErrorsRemoved, nil
}
