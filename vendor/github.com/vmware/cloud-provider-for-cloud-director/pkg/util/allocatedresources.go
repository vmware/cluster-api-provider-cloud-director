package util

import (
	swaggerClient "github.com/vmware/cloud-provider-for-cloud-director/pkg/vcdswaggerclient_36_0"
	"sync"
)

type AllocatedResourcesMap struct {
	RWLock             sync.RWMutex
	allocatedResources map[string][]swaggerClient.EntityReference
}

func (ar *AllocatedResourcesMap) Insert(key string, value *swaggerClient.EntityReference) {
	if value == nil {
		return
	}

	ar.RWLock.Lock()
	defer ar.RWLock.Unlock()

	if ar.allocatedResources == nil {
		ar.allocatedResources = make(map[string][]swaggerClient.EntityReference)
	}

	if _, ok := ar.allocatedResources[key]; !ok {
		ar.allocatedResources[key] = make([]swaggerClient.EntityReference, 1)
		ar.allocatedResources[key][0] = *value
		return
	}

	// Add the value if it doesn't exist into the list
	// This is ideally best implemented using a bst or another tree, but we will just do a dumb linear search
	// since these structures are tiny.
	for idx, entityReference := range ar.allocatedResources[key] {
		if entityReference.Name == value.Name {
			ar.allocatedResources[key][idx].Id = value.Id
			return
		}
	}

	ar.allocatedResources[key] = append(ar.allocatedResources[key], *value)

	return
}

func (ar *AllocatedResourcesMap) Get(key string) []swaggerClient.EntityReference {
	ar.RWLock.RLock()
	defer ar.RWLock.RUnlock()

	values, ok := ar.allocatedResources[key]
	if !ok {
		return nil
	}

	return values
}
