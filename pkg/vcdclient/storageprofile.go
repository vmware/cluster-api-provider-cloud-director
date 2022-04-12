/*
   Copyright 2021 VMware, Inc.
   SPDX-License-Identifier: Apache-2.0
*/

package vcdclient

import (
	"fmt"
	"github.com/vmware/go-vcloud-director/v2/types/v56"
)

func (client *Client) GetStorageProfileDetailsFromName(storageProfileName string) (*types.QueryResultProviderVdcStorageProfileRecordType, error) {
	storageProfileRecord, err := client.VcdClient.QueryProviderVdcStorageProfileByName(storageProfileName, client.Vdc.Vdc.HREF)
	if err != nil {
		return nil, fmt.Errorf("unable to get storage profile [%s] by name: [%v]", storageProfileName, err)
	}
	return storageProfileRecord, nil
}
