package vcdsdk

import (
	"fmt"
	swaggerClient "github.com/vmware/cloud-provider-for-cloud-director/pkg/vcdswaggerclient"
)

type CapvcdRdeFoundError struct {
	EntityType string
}

func (e CapvcdRdeFoundError) Error() string {
	return fmt.Sprintf("found entity of type [%s]", e.EntityType)
}

type CPIStatus struct {
	Name           string         `json:"name,omitempty"`
	Version        string         `json:"version,omitempty"`
	VCDResourceSet []VCDResource  `json:"vcdResourceSet,omitempty"`
	Errors         []BackendError `json:"errorSet,omitempty"`
	Events         []BackendEvent `json:"eventSet,omitempty"`
	VirtualIPs     []string       `json:"virtualIPs,omitempty"`
}

func GetVirtualIPsFromRDE(rde *swaggerClient.DefinedEntity) ([]string, error) {
	statusEntry, ok := rde.Entity["status"]
	if !ok {
		return nil, fmt.Errorf("could not find 'status' entry in defined entity")
	}
	statusMap, ok := statusEntry.(map[string]interface{})
	if !ok {
		return nil, fmt.Errorf("unable to convert [%T] to map", statusEntry)
	}

	var virtualIpInterfaces interface{}
	if IsCAPVCDEntityType(rde.EntityType) {
		return nil, CapvcdRdeFoundError{
			EntityType: rde.EntityType,
		}
	} else if IsNativeClusterEntityType(rde.EntityType) {
		virtualIpInterfaces = statusMap["virtual_IPs"]
	} else {
		return nil, fmt.Errorf("entity type %s not supported by CPI", rde.EntityType)
	}

	if virtualIpInterfaces == nil {
		return make([]string, 0), nil
	}

	virtualIpInterfacesSlice, ok := virtualIpInterfaces.([]interface{})
	if !ok {
		return nil, fmt.Errorf("unable to convert [%T] to slice of interface", virtualIpInterfaces)
	}
	virtualIpStrs := make([]string, len(virtualIpInterfacesSlice))
	for ind, ipInterface := range virtualIpInterfacesSlice {
		currIp, ok := ipInterface.(string)
		if !ok {
			return nil, fmt.Errorf("unable to convert [%T] to string", ipInterface)
		}
		virtualIpStrs[ind] = currIp
	}
	return virtualIpStrs, nil
}

// ReplaceVirtualIPsInRDE replaces the virtual IPs array in the inputted rde. It does not make an API call to update
// the RDE.
func ReplaceVirtualIPsInRDE(rde *swaggerClient.DefinedEntity, updatedIps []string) (*swaggerClient.DefinedEntity, error) {
	statusEntry, ok := rde.Entity["status"]
	if !ok {
		return nil, fmt.Errorf("could not find 'status' entry in defined entity")
	}
	statusMap, ok := statusEntry.(map[string]interface{})
	if !ok {
		return nil, fmt.Errorf("unable to convert [%T] to map", statusEntry)
	}
	if IsCAPVCDEntityType(rde.EntityType) {
		capvcdEntityFoundErr := CapvcdRdeFoundError{
			EntityType: rde.EntityType,
		}
		return nil, capvcdEntityFoundErr
	} else if IsNativeClusterEntityType(rde.EntityType) {
		statusMap["virtual_IPs"] = updatedIps
	}
	return rde, nil
}
