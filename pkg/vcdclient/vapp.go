/*
   Copyright 2021 VMware, Inc.
   SPDX-License-Identifier: Apache-2.0
*/

package vcdclient

import (
	"fmt"
	"github.com/vmware/go-vcloud-director/v2/govcd"
	"github.com/vmware/go-vcloud-director/v2/types/v56"
	"k8s.io/klog/v2"
)

type VdcManager struct {
	OrgName string
	VdcName string
	Vdc     *govcd.Vdc
	Client  *Client
}

func (vdc *VdcManager) CacheVdcDetails() error {

	vcdClient, err := vdc.Client.VcdAuthConfig.GetPlainClientFromSecrets()
	if err != nil {
		return fmt.Errorf("unable to get plain client from secrets: [%v]", err)
	}

	org, err := vcdClient.GetOrgByName(vdc.OrgName)
	if err != nil {
		return fmt.Errorf("unable to get org from name [%s]: [%v]", vdc.OrgName, err)
	}

	vdc.Vdc, err = org.GetVDCByName(vdc.VdcName, true)
	if err != nil {
		return fmt.Errorf("unable to get Vdc [%s] from org [%s]: [%v]", vdc.VdcName, vdc.OrgName, err)
	}
	return nil
}

func (vdc *VdcManager) FindAllVMsInVapp(vAppName string) ([]*types.Vm, error) {

	vApp, err := vdc.Vdc.GetVAppByName(vAppName, true)
	if err != nil {
		return nil, fmt.Errorf("unable to find vApp [%s]: [%v]", vAppName, err)
	}

	if vApp.VApp == nil {
		return nil, fmt.Errorf("unable to get VApp object in vapp of name [%s]", vAppName)
	}

	if vApp.VApp.Children == nil {
		return nil, nil
	}

	return vApp.VApp.Children.VM, nil
}

func (vdc *VdcManager) DeleteVApp(vAppName string) error {
	vdc.Client.rwLock.Lock()
	defer vdc.Client.rwLock.Unlock()

	if vdc.Vdc == nil {
		return fmt.Errorf("no Vdc created with name [%s]", vdc.VdcName)
	}
	vApp, err := vdc.Vdc.GetVAppByName(vAppName, true)
	if err != nil {
		return govcd.ErrorEntityNotFound
	}
	task, err := vApp.Undeploy()
	if err != nil {
		task, err = vApp.Delete()
		if err != nil {
			return fmt.Errorf("failed to delete vApp [%s]", vAppName)
		}
	}
	err = task.WaitTaskCompletion()
	if err != nil {
		return fmt.Errorf("error performing undeploy vApp task: %s", err)
	}
	task, err = vApp.Delete()
	if err != nil {
		return fmt.Errorf("failed to delete vApp [%s]", vAppName)
	}
	err = task.WaitTaskCompletion()
	if err != nil {
		return fmt.Errorf("error performing delete vApp task: %s", err)
	}
	return nil
}

// no need to make reentrant since VCD will take care of it and Kubernetes will retry
func (vdc *VdcManager) GetOrCreateVApp(vAppName string, ovdcNetworkName string) (*govcd.VApp, error) {
	if vdc.Vdc == nil {
		return nil, fmt.Errorf("no Vdc created with name [%s]", vdc.VdcName)
	}

	vApp, err := vdc.Vdc.GetVAppByName(vAppName, true)
	if err != nil && err != govcd.ErrorEntityNotFound {
		return nil, fmt.Errorf("unable to get vApp [%s] from Vdc [%s]: [%v]",
			vAppName, vdc.VdcName, err)
	} else if vApp != nil {
		if vApp.VApp == nil {
			return nil, fmt.Errorf("vApp [%s] is invalid", vAppName)
		}
		if !vdc.isVappNetworkPresentInVapp(vApp, ovdcNetworkName) {
			// try adding ovdc network to the vApp
			if err := vdc.addOvdcNetworkToVApp(vApp, ovdcNetworkName); err != nil {
				return nil, fmt.Errorf("unable to add ovdc network [%s] to vApp [%s]: [%v]", ovdcNetworkName, vAppName, err)
			}
			klog.V(3).Infof("successfully added ovdc network [%s] to vApp [%s]", ovdcNetworkName, vAppName)
		}
		return vApp, nil
	}

	// vapp not found, so create one
	err = vdc.Vdc.ComposeRawVApp(vAppName, fmt.Sprintf("Description for [%s]", vAppName))
	if err != nil {
		return nil, fmt.Errorf("unable to compose raw vApp with name [%s]: [%v]", vAppName, err)
	}

	vApp, err = vdc.Vdc.GetVAppByName(vAppName, true)
	if err != nil {
		return nil, fmt.Errorf("unable to get vApp [%s] from Vdc [%s]: [%v]",
			vAppName, vdc.VdcName, err)
	}

	if err := vdc.addOvdcNetworkToVApp(vApp, ovdcNetworkName); err != nil {
		return nil, fmt.Errorf("unable to add ovdc network [%s] to vApp [%s]: [%v]", ovdcNetworkName, vAppName, err)
	}

	return vApp, nil
}

func (vdc *VdcManager) addOvdcNetworkToVApp(vApp *govcd.VApp, ovdcNetworkName string) error {
	if vApp == nil || vApp.VApp == nil {
		return fmt.Errorf("cannot add ovdc network to a nil vApp")
	}
	ovdcNetwork, err := vdc.Vdc.GetOrgVdcNetworkByName(ovdcNetworkName, true)
	if err != nil {
		return fmt.Errorf("unable to get ovdc network [%s]: [%v]", ovdcNetworkName, err)
	}
	_, err = vApp.AddOrgNetwork(&govcd.VappNetworkSettings{}, ovdcNetwork.OrgVDCNetwork,
		false)
	if err != nil {
		return fmt.Errorf("unable to add ovdc network [%s] to vApp [%s]: [%v]",
			ovdcNetworkName, vApp.VApp.Name, err)
	}
	return nil
}

func (vdc *VdcManager) AddMetadataToVApp(vAppName string, paramMap map[string]string) error {
	vApp, err := vdc.Vdc.GetVAppByName(vAppName, true)
	if err != nil && err != govcd.ErrorEntityNotFound {
		return fmt.Errorf("unable to get vApp [%s] from Vdc [%s]: [%v]",
			vAppName, vdc.VdcName, err)
	}
	if vApp == nil || vApp.VApp == nil {
		return fmt.Errorf("cannot add metadata to a nil vApp")
	}
	for key, value := range paramMap {
		_, err := vApp.AddMetadata(key, value)
		if err != nil {
			return fmt.Errorf("unable to add metadata  [%s]: [%s] to vApp [%s]: [%v]",
				key, value, vApp.VApp.Name, err)
		}
	}
	return nil
}

func (vdc *VdcManager) isVappNetworkPresentInVapp(vApp *govcd.VApp, ovdcNetworkName string) bool {
	if vApp == nil || vApp.VApp == nil {
		klog.Error("found nil value for vApp")
		return false
	}
	if vApp.VApp.NetworkConfigSection != nil && vApp.VApp.NetworkConfigSection.NetworkConfig != nil {
		for _, vAppNetwork := range vApp.VApp.NetworkConfigSection.NetworkNames() {
			if vAppNetwork == ovdcNetworkName {
				return true
			}
		}
	}
	return false
}
