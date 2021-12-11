/*
   Copyright 2021 VMware, Inc.
   SPDX-License-Identifier: Apache-2.0
*/

package vcdclient

import (
	"fmt"
	"github.com/vmware/go-vcloud-director/v2/govcd"
)

func (client *Client) GetCatalogByName(orgName string, catalogName string) (*govcd.Catalog, error) {
	org, err := client.VcdClient.GetOrgByName(client.ClusterOrgName)
	if err != nil {
		return nil, fmt.Errorf("unable to get vcd organization [%s]: [%v]", client.ClusterOrgName, err)
	}
	if err := org.Refresh(); err != nil {
		return nil, fmt.Errorf("unable to refresh org [%s]: [%v]", orgName, err)
	}
	catalog, err := org.GetCatalogByName(catalogName, true)
	if err != nil {
		return catalog, fmt.Errorf("unable to find catalog [%s] in org [%s]", catalogName, orgName)
	}
	return catalog, nil
}
