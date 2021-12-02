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

func TestVMCreation(t *testing.T) {

	// get client
	vcdClient, err := getTestVCDClient(
		map[string]interface{}{
			"getVdcClient": true,
		})
	assert.NoError(t, err, "Unable to get VCD client")
	require.NotNil(t, vcdClient, "VCD Client should not be nil")

	// create vApp
	vAppName := "test-vapp"
	vdcManager := VdcManager{
		VdcName: vcdClient.VcdAuthConfig.VDC,
		OrgName: vcdClient.VcdAuthConfig.Org,
		Client:  vcdClient,
		Vdc:     vcdClient.Vdc,
	}
	vApp, err := vdcManager.GetOrCreateVApp(vAppName, "tenant1_ovdc_nw")
	assert.NoError(t, err, "unable to create vApp")
	require.NotNil(t, vApp, "vApp created should not be nil")

	// create vm
	vmNamePrefix := "test-vm-1"
	vmNum := 1
	guestCustScript := `
#!/usr/bin/env bash -x
echo "Command called with arguments [$@] at time $(date)" >> /root/output.txt
if [ -f "/.guest-customization-post-reboot-pending" ]
then
	echo "Reboot pending, hence will do nothing."  >> /root/output.txt
elif [ "$1" = "postcustomization" ]
then
	kubeadm init --ttl=0
	join_token = $(kubeadm token create --print-join-command)
	vmtoolsd --cmd "info-set guestinfo.joinToken "${join_token}"
else
	echo "Skipping script since postcustomization is not involved." >> /root/output.txt
fi
exit 0
`
	_, err = vdcManager.AddNewMultipleVM(vApp, vmNamePrefix, vmNum, "ProviderCatalogs",
		"ubuntu-16.04_k8-1.20_weave-2.6.5_rev2",
		"cse----native", "2core2gb", guestCustScript, true, true)
	require.NoError(t, err, "unable to create [%d] VMs", vmNum)

	_ = vdcManager.WaitForGuestScriptCompletion(vmNamePrefix, vAppName)

	//err = VcdClient.DeleteVM(vAppName, vmName)
	//assert.NoError(t, err, "unable to delete VM")

	return
}

func TestVMExtraConfig(t *testing.T) {
	// get client
	vcdClient, err := getTestVCDClient(
		map[string]interface{}{
			"getVdcClient": true,
		})
	assert.NoError(t, err, "Unable to get VCD client")
	require.NotNil(t, vcdClient, "VCD Client should not be nil")

	// create vApp
	vAppName := "test-vapp"
	vdcManager := VdcManager{
		VdcName: vcdClient.VcdAuthConfig.VDC,
		OrgName: vcdClient.VcdAuthConfig.Org,
		Client:  vcdClient,
		Vdc:     vcdClient.Vdc,
	}
	vApp, err := vdcManager.GetOrCreateVApp(vAppName, vcdClient.NetworkName)
	assert.NoError(t, err, "unable to create vApp")
	require.NotNil(t, vApp, "vApp created should not be nil")

	// create vm
	vmNamePrefix := "test-vm-1"
	vmNum := 1
	guestCustScript := `
#!/usr/bin/env bash -x
echo "Command called with arguments [$@] at time $(date)" >> /root/output.txt
if [ -f "/.guest-customization-post-reboot-pending" ]
then
	echo "Reboot pending, hence will do nothing."  >> /root/output.txt
elif [ "$1" = "postcustomization" ]
then
	kubeadm init --ttl=0
	join_token = $(kubeadm token create --print-join-command)
	vmtoolsd --cmd "info-set guestinfo.joinToken "${join_token}"
else
	echo "Skipping script since postcustomization is not involved." >> /root/output.txt
fi
exit 0
`

	// TODO: allow these vm params to be user passed through a config
	err = vdcManager.AddNewVM(vmNamePrefix, vAppName, vmNum, "cse",
		"ubuntu-16.04_k8-1.21_weave-2.8.1_rev1", "cse----native",
		"2core2gb", guestCustScript, true)
	assert.NoError(t, err, "unable to create [%d] VMs", vmNum)

	vms, err := vdcManager.FindAllVMsInVapp(vAppName)
	assert.NoError(t, err, "unable to find VMs in vApp")
	assert.NotNil(t, vms, "some VMs should be returned")
	assert.True(t, len(vms) == vmNum, "vapp should have [%d] vm(s), but has %d vm(s)", vmNum, len(vms))

	vm, err := vApp.GetVMByName(vms[0].Name, true)
	assert.NoError(t, err, "failed to get vm")

	// test extra config
	key := "extraconfig.test"
	value := "test123"
	err = vcdClient.SetVmExtraConfigKeyValue(vm, key, value, false)
	assert.NoError(t, err, "error setting VM extra config: [%v]", err)

	retrievedValue, err := vcdClient.GetExtraConfigValue(vm, key)
	assert.NoError(t, err, "failed to get hardware section value for key %s", key)
	assert.Equal(t, value, retrievedValue, "retrieved incorrect value")

	err = vdcManager.DeleteVApp(vAppName)
	assert.NoError(t, err, "unable to delete vApp: [%s]", vAppName)
}
