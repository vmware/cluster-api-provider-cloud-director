package vcdclient

import (
	"fmt"
	"github.com/vmware/go-vcloud-director/v2/govcd"
	"github.com/vmware/go-vcloud-director/v2/types/v56"
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

	if err := vdc.Client.RefreshToken(); err != nil {
		return fmt.Errorf("unable to refresh token in GetVApp for [%s]: [%v", vAppName, err)
	}
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

// make reentrant
func (vdc *VdcManager) GetOrCreateVApp(vAppName string, ovdcNetworkName string) (*govcd.VApp, error) {
	if err := vdc.Client.RefreshToken(); err != nil {
		return nil, fmt.Errorf("unable to refresh token in GetVApp for [%s]: [%v", vAppName, err)
	}

	if vdc.Vdc == nil {
		return nil, fmt.Errorf("no Vdc created with name [%s]", vdc.VdcName)
	}

	vApp, err := vdc.Vdc.GetVAppByName(vAppName, true)
	if err != nil && err != govcd.ErrorEntityNotFound {
		return nil, fmt.Errorf("unable to get vApp [%s] from Vdc [%s]: [%v]",
			vAppName, vdc.VdcName, err)
	} else if vApp != nil {
		if vApp.VApp == nil || vApp.VApp.NetworkConfigSection == nil {
			return nil, fmt.Errorf("vApp [%s] already present but no network configured", vAppName)
		}
		networkFound := false
		for _, networkName := range vApp.VApp.NetworkConfigSection.NetworkNames() {
			if networkName == ovdcNetworkName {
				networkFound = true
				break
			}
		}
		if !networkFound {
			return nil, fmt.Errorf("vApp [%s] already present but network [%s] not found",
				vAppName, ovdcNetworkName)
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

	ovdcNetwork, err := vdc.Vdc.GetOrgVdcNetworkByName(ovdcNetworkName, true)
	if err != nil {
		return nil, fmt.Errorf("unable to get ovdc network [%s]: [%v]", ovdcNetworkName, err)
	}

	_, err = vApp.AddOrgNetwork(&govcd.VappNetworkSettings{}, ovdcNetwork.OrgVDCNetwork,
		false)
	if err != nil {
		return nil, fmt.Errorf("unable to add ovdc network [%s] to vApp [%s]: [%v]",
			ovdcNetworkName, vAppName, err)
	}

	return vApp, nil
}
