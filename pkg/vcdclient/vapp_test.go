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

var (
	//CAPVCDEntityTypeID = fmt.Sprintf("urn:vcloud:type:%s:%s:%s", CAPVCDTypeVendor, CAPVCDTypeNss, CAPVCDTypeVersion)
	InfraID = "InfraID"
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
		VdcName: vcdClient.ClusterOVDCName,
		OrgName: vcdClient.ClusterOrgName,
		Client:  vcdClient,
		Vdc:     vcdClient.Vdc,
	}
	vms, err := vdcManager.FindAllVMsInVapp(vAppName)
	assert.NoError(t, err, "unable to find VMs in vApp")
	assert.NotNil(t, vms, "some VMs should be returned")

	return
}

func TestVAppMetaData(t *testing.T) {

	// get client
	vcdClient, err := getTestVCDClient(
		map[string]interface{}{
			"getVdcClient": true,
		})
	assert.NoError(t, err, "Unable to get VCD client")
	require.NotNil(t, vcdClient, "VCD Client should not be nil")

	// create vApp
	vAppName := "capi-cluster-3"
	vdcManager := VdcManager{
		VdcName: vcdClient.ClusterOVDCName,
		OrgName: vcdClient.ClusterOrgName,
		Client:  vcdClient,
		Vdc:     vcdClient.Vdc,
	}
	//matadataMap := map[string]string{
	//	InfraID: "testuse",
	//}
	_, err = vdcManager.GetOrCreateVApp(vAppName, "ovdc1_nw", nil)
	if err != nil {
		return
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
		VdcName: vcdClient.ClusterOVDCName,
		OrgName: vcdClient.ClusterOrgName,
		Client:  vcdClient,
		Vdc:     vcdClient.Vdc,
	}
	vapp, err := vdcManager.GetOrCreateVApp("vapp1", "ovdc1_nw", nil)
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
		VdcName: vcdClient.ClusterOVDCName,
		OrgName: vcdClient.ClusterOrgName,
		Client:  vcdClient,
		Vdc:     vcdClient.Vdc,
	}
	vdcManager.CacheVdcDetails()
}
