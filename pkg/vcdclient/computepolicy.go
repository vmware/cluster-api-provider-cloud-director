/*
   Copyright 2021 VMware, Inc.
   SPDX-License-Identifier: Apache-2.0
*/

package vcdclient

import (
	"fmt"
	"github.com/go-openapi/errors"
	"github.com/vmware/go-vcloud-director/v2/types/v56"
	"net/url"
)

func (client *Client) GetComputePolicyDetailsFromName(computePolicyName string) (*types.VdcComputePolicy, error) {
	org, err := client.VcdClient.GetOrgByName(client.ClusterOrgName)
	if err != nil {
		return nil, fmt.Errorf("unable to get org [%s] by name: [%v]", client.ClusterOrgName, err)
	}

	vdcComputePolicies, err := org.GetAllVdcComputePolicies(url.Values{})
	if err != nil {
		return nil, fmt.Errorf("unable to get all compute policies for [%s] by name: [%v]",
			client.ClusterOrgName, err)
	}

	var computePolicy *types.VdcComputePolicy = nil
	for _, vdcComputePolicy := range vdcComputePolicies {
		if vdcComputePolicy.VdcComputePolicy == nil {
			continue
		}
		if vdcComputePolicy.VdcComputePolicy.Name == computePolicyName {
			computePolicy = vdcComputePolicy.VdcComputePolicy
			break
		}
	}

	if computePolicy == nil {
		return nil, errors.NotFound("unable to find compute policy [%s]", computePolicyName)
	}

	return computePolicy, nil
}
