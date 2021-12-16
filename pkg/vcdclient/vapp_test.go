/*
   Copyright 2021 VMware, Inc.
   SPDX-License-Identifier: Apache-2.0
*/

package vcdclient

import (
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"testing"
)

func TestVApp(t *testing.T) {

	// get client
	vcdClient, err := getTestVCDClient(
		map[string]interface{}{
			"getVdcClient": true,
		})
	assert.NoError(t, err, "Unable to get VCD client")
	require.NotNil(t, vcdClient, "VCD Client should not be nil")

	// create vApp
	vAppName := "manual-vapp"
	vdcManager := VdcManager{
		VdcName: vcdClient.VcdAuthConfig.VDC,
		OrgName: vcdClient.VcdAuthConfig.UserOrg,
		Client:  vcdClient,
		Vdc:     vcdClient.Vdc,
	}
	vms, err := vdcManager.FindAllVMsInVapp(vAppName)
	assert.NoError(t, err, "unable to find VMs in vApp")
	assert.NotNil(t, vms, "some VMs should be returned")

	return
}

func TestDeleteVapp(t *testing.T) {
	// get client
	vcdClient, err := getTestVCDClient(
		map[string]interface{}{
			"getVdcClient": true,
		})
	assert.NoError(t, err, "Unable to get VCD client")
	require.NotNil(t, vcdClient, "VCD Client should not be nil")

	vdcManager := VdcManager{
		VdcName: vcdClient.VcdAuthConfig.VDC,
		OrgName: vcdClient.VcdAuthConfig.UserOrg,
		Client:  vcdClient,
		Vdc:     vcdClient.Vdc,
	}
	vapp, err := vdcManager.GetOrCreateVApp("vapp1", "ovdc1_nw")
	assert.NoError(t, err, "unable to find vApp")
	assert.NotNil(t, vapp, "vapp should not be nil")
	err = vdcManager.DeleteVApp("vapp1")
	assert.NoError(t, err, "unable to delete vApp")
}

func TestVdcManager_CacheVdcDetails(t *testing.T) {
	vcdClient, err := getTestVCDClient(
		map[string]interface{}{
			"getVdcClient": true,
		})
	assert.NoError(t, err, "Unable to get VCD client")
	require.NotNil(t, vcdClient, "VCD Client should not be nil")
	vdcManager := VdcManager{
		VdcName: vcdClient.VcdAuthConfig.VDC,
		OrgName: vcdClient.VcdAuthConfig.UserOrg,
		Client:  vcdClient,
		Vdc:     vcdClient.Vdc,
	}
	vdcManager.CacheVdcDetails()
}
