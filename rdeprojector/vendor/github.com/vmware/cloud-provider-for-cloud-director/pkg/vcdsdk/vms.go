/*
   Copyright 2021 VMware, Inc.
   SPDX-License-Identifier: Apache-2.0
*/

package vcdsdk

import (
	"fmt"
	"k8s.io/klog"
	"strings"

	"github.com/vmware/go-vcloud-director/v2/govcd"
)

const (
	// VCDVMIDPrefix is a prefix added to VM objects by VCD. This needs
	// to be removed for query operations.
	VCDVMIDPrefix = "urn:vcloud:vm:"
)

type VMManager struct {
	Client          *Client
	ClusterVAppName string
}

func NewVMManager(client *Client, clusterVAppName string) *VMManager {
	return &VMManager{
		Client:          client,
		ClusterVAppName: clusterVAppName,
	}
}

// FindVMByName finds a VM in a vApp using the name. The client is expected to have a valid
// bearer token when this function is called.
func (vmm *VMManager) FindVMByName(vmName string) (*govcd.VM, error) {
	if vmName == "" {
		return nil, fmt.Errorf("vmName mandatory for FindVMByName")
	}

	client := vmm.Client
	klog.Infof("Trying to find vm [%s] in vApp [%s] by name", vmName, vmm.ClusterVAppName)
	vApp, err := client.VDC.GetVAppByName(vmm.ClusterVAppName, true)
	if err != nil {
		return nil, fmt.Errorf("unable to find vApp [%s] by name: [%v]", vmm.ClusterVAppName, err)
	}

	vm, err := vApp.GetVMByName(vmName, true)
	if err != nil {
		return nil, fmt.Errorf("unable to find vm [%s] in vApp [%s]: [%v]", vmName, vmm.ClusterVAppName, err)
	}

	return vm, nil
}

// FindVMByUUID finds a VM in a vApp using the UUID. The client is expected to have a valid
// bearer token when this function is called.
func (vmm *VMManager) FindVMByUUID(vcdVmUUID string) (*govcd.VM, error) {
	if vcdVmUUID == "" {
		return nil, fmt.Errorf("vmUUID mandatory for FindVMByUUID")
	}

	klog.Infof("Trying to find vm [%s] in vApp [%s] by UUID", vcdVmUUID, vmm.ClusterVAppName)
	vmUUID := strings.TrimPrefix(vcdVmUUID, VCDVMIDPrefix)

	vApp, err := vmm.Client.VDC.GetVAppByName(vmm.ClusterVAppName, true)
	if err != nil {
		return nil, fmt.Errorf("unable to find vApp [%s] by name: [%v]", vmm.ClusterVAppName, err)
	}

	vm, err := vApp.GetVMById(vmUUID, true)
	if err != nil {
		return nil, fmt.Errorf("unable to find vm UUID [%s] in vApp [%s]: [%v]",
			vmUUID, vmm.ClusterVAppName, err)
	}

	return vm, nil
}

// IsVmNotAvailable : In VCD, if the VM is not available, it can be an access error or the VM may not be present.
// Hence we sometimes get an error different from govcd.ErrorEntityNotFound
func (vmm *VMManager) IsVmNotAvailable(err error) bool {

	if strings.Contains(err.Error(), "Either you need some or all of the following rights [Base]") &&
		strings.Contains(err.Error(), "to perform operations [VAPP_VM_VIEW]") &&
		strings.Contains(err.Error(), "target entity is invalid") {
		return true
	}

	if strings.Contains(err.Error(), "error refreshing VM: cannot refresh VM, Object is empty") {
		return true
	}

	return false
}
