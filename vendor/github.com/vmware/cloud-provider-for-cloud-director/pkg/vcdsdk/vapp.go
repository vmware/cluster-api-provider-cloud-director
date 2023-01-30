/*
   Copyright 2021 VMware, Inc.
   SPDX-License-Identifier: Apache-2.0
*/

package vcdsdk

import (
	"bytes"
	"encoding/xml"
	"fmt"
	"github.com/vmware/go-vcloud-director/v2/govcd"
	"github.com/vmware/go-vcloud-director/v2/types/v56"
	"io/ioutil"
	"k8s.io/klog/v2"
	"net/http"
	"net/url"
	"reflect"
	"strconv"
	"strings"
	"sync"
	"time"
)

const (
	// VCDVMIDPrefix is a prefix added to VM objects by VCD. This needs
	// to be removed for query operations.
	VCDVMIDPrefix = "urn:vcloud:vm:"
)

var (
	trueVar  = true
	falseVar = false
)

type VdcManager struct {
	OrgName string
	VdcName string
	Vdc     *govcd.Vdc
	// client should be refreshed
	Client *Client
}

func convertConnectionToMarshalConnection(connections []*types.VirtualHardwareConnection) []*VirtualHardwareConnectionMarshal {
	marshalConnections := make([]*VirtualHardwareConnectionMarshal, len(connections))
	for i, connection := range connections {
		marshalConnections[i] = &VirtualHardwareConnectionMarshal{
			IPAddress:         connection.IPAddress,
			PrimaryConnection: connection.PrimaryConnection,
			IpAddressingMode:  connection.IpAddressingMode,
			Value:             connection.NetworkName,
		}
	}
	return marshalConnections
}

func convertHostResourceToMarshalHostResource(hostResources []*VirtualHardwareHostResource) []*VirtualHardwareHostResourceMarshal {
	marshalHostResources := make([]*VirtualHardwareHostResourceMarshal, len(hostResources))
	for i, hostResource := range hostResources {
		marshalHostResources[i] = &VirtualHardwareHostResourceMarshal{
			BusType:           hostResource.BusType,
			BusSubType:        hostResource.BusSubType,
			Capacity:          hostResource.Capacity,
			Iops:              hostResource.Iops,
			StorageProfile:    hostResource.StorageProfile,
			OverrideVmDefault: hostResource.OverrideVmDefault,
		}
	}
	return marshalHostResources
}

func getNillableElement(item *VirtualHardwareItem, field string) *NillableElementMarshal {
	r := reflect.ValueOf(item)
	foundField := reflect.Indirect(r).FieldByName(field)
	element := foundField.Interface().(*NillableElement)
	if element == nil {
		return &NillableElementMarshal{
			Value:    "",
			XsiNil:   "true",
			XmlnsXsi: "",
		}
	}

	xsiNilValue := "true"
	if element.Value != "" {
		xsiNilValue = ""
	}
	return &NillableElementMarshal{
		Value:    element.Value,
		XsiNil:   xsiNilValue,
		XmlnsXsi: element.XmlnsXsi,
	}
}

func convertItemsToMarshalItems(items []*VirtualHardwareItem) []*VirtualHardwareItemMarshal {
	marshalItems := make([]*VirtualHardwareItemMarshal, len(items))
	for i, item := range items {
		var coresPerSocketMarshal *CoresPerSocketMarshal = nil
		if item.CoresPerSocket != nil {
			coresPerSocketMarshal = &CoresPerSocketMarshal{
				OvfRequired: item.CoresPerSocket.OvfRequired,
				Value:       item.CoresPerSocket.Value,
			}
		}
		marshalItems[i] = &VirtualHardwareItemMarshal{
			Href:                  item.Href,
			Type:                  item.Type,
			ResourceType:          item.ResourceType,
			ResourceSubType:       getNillableElement(item, "ResourceSubType"),
			ElementName:           getNillableElement(item, "ElementName"),
			Description:           getNillableElement(item, "Description"),
			InstanceID:            item.InstanceID,
			ConfigurationName:     getNillableElement(item, "ConfigurationName"),
			ConsumerVisibility:    getNillableElement(item, "ConsumerVisibility"),
			AutomaticAllocation:   getNillableElement(item, "AutomaticAllocation"),
			AutomaticDeallocation: getNillableElement(item, "AutomaticDeallocation"),
			Address:               getNillableElement(item, "Address"),
			AddressOnParent:       getNillableElement(item, "AddressOnParent"),
			AllocationUnits:       getNillableElement(item, "AllocationUnits"),
			Reservation:           getNillableElement(item, "Reservation"),
			VirtualQuantity:       getNillableElement(item, "VirtualQuantity"),
			VirtualQuantityUnits:  getNillableElement(item, "VirtualQuantityUnits"),
			Weight:                getNillableElement(item, "Weight"),
			CoresPerSocket:        coresPerSocketMarshal,
			Connection:            convertConnectionToMarshalConnection(item.Connection),
			HostResource:          convertHostResourceToMarshalHostResource(item.HostResource),
			Link:                  item.Link,
			Parent:                getNillableElement(item, "Parent"),
			Generation:            getNillableElement(item, "Generation"),
			Limit:                 getNillableElement(item, "Limit"),
			MappingBehavior:       getNillableElement(item, "MappingBehavior"),
			OtherResourceType:     getNillableElement(item, "OtherResourceType"),
			PoolID:                getNillableElement(item, "PoolID"),
		}
	}
	return marshalItems
}

func NewVDCManager(client *Client, orgName string, vdcName string) (*VdcManager, error) {
	if orgName == "" {
		orgName = client.ClusterOrgName
	}
	if vdcName == "" {
		vdcName = client.ClusterOVDCName
	}

	vdcManager := &VdcManager{
		Client:  client,
		OrgName: orgName,
		VdcName: vdcName,
	}
	err := vdcManager.cacheVdcDetails()
	if err != nil {
		return nil, fmt.Errorf("failed to cache VDC details: [%v]", err)
	}
	return vdcManager, nil
}

func (vdc *VdcManager) cacheVdcDetails() error {
	org, err := vdc.Client.VCDClient.GetOrgByName(vdc.OrgName)
	if err != nil {
		return fmt.Errorf("unable to get org from name [%s]: [%v]", vdc.OrgName, err)
	}

	vdc.Vdc, err = org.GetVDCByName(vdc.VdcName, true)
	if err != nil {
		return fmt.Errorf("unable to get Vdc [%s] from org [%s]: [%v]", vdc.VdcName, vdc.OrgName, err)
	}
	return nil
}

func (vdc *VdcManager) FindAllVMsInVapp(VAppName string) ([]*types.Vm, error) {

	if VAppName == "" {
		return nil, fmt.Errorf("VApp name is empty")
	}

	vApp, err := vdc.Vdc.GetVAppByName(VAppName, true)
	if err != nil {
		return nil, fmt.Errorf("unable to find vApp [%s]: [%v]", VAppName, err)
	}

	if vApp.VApp == nil {
		return nil, fmt.Errorf("unable to get VApp object in vapp of name [%s]", VAppName)
	}

	if vApp.VApp.Children == nil {
		return nil, nil
	}

	return vApp.VApp.Children.VM, nil
}

func (vdc *VdcManager) DeleteVApp(VAppName string) error {
	vdc.Client.RWLock.Lock()
	defer vdc.Client.RWLock.Unlock()

	if vdc.Vdc == nil {
		return fmt.Errorf("no Vdc created with name [%s]", vdc.Client.ClusterOVDCName)
	}
	vApp, err := vdc.Vdc.GetVAppByName(VAppName, true)
	if err != nil {
		return govcd.ErrorEntityNotFound
	}
	task, err := vApp.Undeploy()
	if err != nil {
		task, err = vApp.Delete()
		if err != nil {
			return fmt.Errorf("failed to delete vApp [%s]: [%v]", VAppName, err)
		}

		// Undeploy can fail if the vApp is not running. But VApp will be in a state where it can be deleted
		err = task.WaitTaskCompletion()
		if err != nil {
			return fmt.Errorf("failed to delete vApp [%s]: [%v]", VAppName, err)
		}
		// Deletion successful
		return nil
	}
	err = task.WaitTaskCompletion()
	if err != nil {
		return fmt.Errorf("error performing undeploy vApp task: %s", err)
	}
	task, err = vApp.Delete()
	if err != nil {
		return fmt.Errorf("failed to delete vApp [%s]: [%v]", VAppName, err)
	}
	err = task.WaitTaskCompletion()
	if err != nil {
		return fmt.Errorf("error performing delete vApp task: %s", err)
	}
	return nil
}

// no need to make reentrant since VCD will take care of it and Kubernetes will retry
func (vdc *VdcManager) GetOrCreateVApp(VAppName string, ovdcNetworkName string) (*govcd.VApp, error) {
	if vdc.Vdc == nil {
		return nil, fmt.Errorf("no Vdc created with name [%s]", vdc.Client.ClusterOVDCName)
	}

	vApp, err := vdc.Vdc.GetVAppByName(VAppName, true)
	if err != nil && err != govcd.ErrorEntityNotFound {
		return nil, fmt.Errorf("unable to get vApp [%s] from Vdc [%s]: [%v]",
			VAppName, vdc.Client.ClusterOVDCName, err)
	} else if vApp != nil {
		if vApp.VApp == nil {
			return nil, fmt.Errorf("vApp [%s] is invalid", VAppName)
		}
		if !vdc.isVappNetworkPresentInVapp(vApp, ovdcNetworkName) {
			// try adding ovdc network to the vApp
			if err := vdc.addOvdcNetworkToVApp(vApp, ovdcNetworkName); err != nil {
				return nil, fmt.Errorf("unable to add ovdc network [%s] to vApp [%s]: [%v]", ovdcNetworkName, VAppName, err)
			}
			klog.V(3).Infof("successfully added ovdc network [%s] to vApp [%s]", ovdcNetworkName, VAppName)
		}
		return vApp, nil
	}

	// vapp not found, so create one
	err = vdc.Vdc.ComposeRawVApp(VAppName, fmt.Sprintf("Description for [%s]", VAppName))
	if err != nil {
		return nil, fmt.Errorf("unable to compose raw vApp with name [%s]: [%v]", VAppName, err)
	}

	vApp, err = vdc.Vdc.GetVAppByName(VAppName, true)
	if err != nil {
		return nil, fmt.Errorf("unable to get vApp [%s] from Vdc [%s]: [%v]",
			VAppName, vdc.Client.ClusterOVDCName, err)
	}

	if err := vdc.addOvdcNetworkToVApp(vApp, ovdcNetworkName); err != nil {
		return nil, fmt.Errorf("unable to add ovdc network [%s] to vApp [%s]: [%v]", ovdcNetworkName, VAppName, err)
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

// FindVMByName finds a VM in a vApp using the name. The client is expected to have a valid
// bearer token when this function is called.
func (vdc *VdcManager) FindVMByName(VAppName string, vmName string) (*govcd.VM, error) {
	if vmName == "" {
		return nil, fmt.Errorf("vmName mandatory for FindVMByName")
	}

	client := vdc.Client
	klog.Infof("Trying to find vm [%s] in vApp [%s] by name", vmName, VAppName)
	vApp, err := client.VDC.GetVAppByName(VAppName, true)
	if err != nil {
		return nil, fmt.Errorf("unable to find vApp [%s] by name: [%v]", VAppName, err)
	}

	vm, err := vApp.GetVMByName(vmName, true)
	if err != nil {
		return nil, fmt.Errorf("unable to find vm [%s] in vApp [%s]: [%v]", vmName, VAppName, err)
	}

	return vm, nil
}

// FindVMByUUID finds a VM in a vApp using the UUID. The client is expected to have a valid
// bearer token when this function is called.
func (vdc *VdcManager) FindVMByUUID(VAppName string, vcdVmUUID string) (*govcd.VM, error) {
	if vcdVmUUID == "" {
		return nil, fmt.Errorf("vmUUID mandatory for FindVMByUUID")
	}

	klog.Infof("Trying to find vm [%s] in vApp [%s] by UUID", vcdVmUUID, VAppName)
	vmUUID := strings.TrimPrefix(vcdVmUUID, VCDVMIDPrefix)

	vApp, err := vdc.Client.VDC.GetVAppByName(VAppName, true)
	if err != nil {
		return nil, fmt.Errorf("unable to find vApp [%s] by name: [%v]", VAppName, err)
	}

	vm, err := vApp.GetVMById(vmUUID, true)
	if err != nil {
		// CPI uses this function via vminfocache which finds VmInfo by using GetByUUID() which calls FindVMByUUID().
		// In CPI, we are handling ErrorEntityNotFound case, but we were not returning it so it skipped this case causing nodes to not get deleted as it was not found.
		if err == govcd.ErrorEntityNotFound {
			return nil, govcd.ErrorEntityNotFound
		}
		return nil, fmt.Errorf("unable to find vm UUID [%s] in vApp [%s]: [%v]",
			vmUUID, VAppName, err)
	}

	return vm, nil
}

// IsVmNotAvailable : In VCD, if the VM is not available, it can be an access error or the VM may not be present.
// Hence we sometimes get an error different from govcd.ErrorEntityNotFound
func (vdc *VdcManager) IsVmNotAvailable(err error) bool {

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

// the returned extra configs is part of the returned vm
func (vdc *VdcManager) getVmExtraConfigs(vm *govcd.VM) ([]*ExtraConfig, *Vm, error) {
	extraConfigVm := &Vm{}

	if vm.VM.HREF == "" {
		return nil, nil, fmt.Errorf("cannot refresh, invalid reference url")
	}

	_, err := vdc.Client.VCDClient.Client.ExecuteRequest(vm.VM.HREF, http.MethodGet,
		"", "error retrieving virtual hardware: %s", nil, extraConfigVm)
	if err != nil {
		return nil, nil, fmt.Errorf("error executing GET request for vm: [%v]", err)
	}

	return extraConfigVm.ExtraConfigVirtualHardwareSection.ExtraConfigs, extraConfigVm, nil
}

func (vdc *VdcManager) GetExtraConfigValue(vm *govcd.VM, key string) (string, error) {
	extraConfigs, _, err := vdc.getVmExtraConfigs(vm)
	if err != nil {
		return "", fmt.Errorf("error retrieving vm extra configs: [%v]", err)
	}

	for _, extraConfig := range extraConfigs {
		if extraConfig.Key == key {
			return extraConfig.Value, nil
		}
	}
	return "", nil
}

func (vdc *VdcManager) getTaskFromResponse(resp *http.Response) (*govcd.Task, error) {
	task := govcd.NewTask(&vdc.Client.VCDClient.Client)
	respBody, err := ioutil.ReadAll(resp.Body)
	defer resp.Body.Close()

	if err != nil {
		return nil, fmt.Errorf("error reading response body: [%v]", err)
	}
	if err = xml.Unmarshal(respBody, &task.Task); err != nil {
		return nil, fmt.Errorf("error unmarshalling response body: [%v]", err)
	}
	return task, nil
}

func (vdc *VdcManager) SetVmExtraConfigKeyValue(vm *govcd.VM, key string, value string, required bool) error {
	_, extraConfigVm, err := vdc.getVmExtraConfigs(vm)
	if err != nil {
		return fmt.Errorf("error retrieving vm extra configs: [%v]", err)
	}
	newExtraConfig := make([]*ExtraConfigMarshal, 1)
	newExtraConfig[0] = &ExtraConfigMarshal{
		Key:      key,
		Value:    value,
		Required: required,
	}

	// form request
	vmMarshal := &VmMarshal{
		Xmlns:                   extraConfigVm.Xmlns,
		Vmext:                   extraConfigVm.Vmext,
		Ovf:                     extraConfigVm.Ovf,
		Vssd:                    extraConfigVm.Vssd,
		Common:                  extraConfigVm.Common,
		Rasd:                    extraConfigVm.Rasd,
		Vmw:                     extraConfigVm.Vmw,
		Ovfenv:                  extraConfigVm.Ovfenv,
		Ns9:                     extraConfigVm.Ns9,
		NeedsCustomization:      extraConfigVm.NeedsCustomization,
		NestedHypervisorEnabled: extraConfigVm.NestedHypervisorEnabled,
		Deployed:                extraConfigVm.Deployed,
		Status:                  extraConfigVm.Status,
		Name:                    extraConfigVm.Name,
		Id:                      extraConfigVm.Id,
		Href:                    extraConfigVm.Href,
		Type:                    extraConfigVm.Type,
		Description:             extraConfigVm.Description,
		VmSpecSection: &VmSpecSectionMarshal{
			Modified:          extraConfigVm.VmSpecSection.Modified,
			Info:              extraConfigVm.VmSpecSection.Info,
			OsType:            extraConfigVm.VmSpecSection.OsType,
			NumCpus:           extraConfigVm.VmSpecSection.NumCpus,
			NumCoresPerSocket: extraConfigVm.VmSpecSection.NumCoresPerSocket,
			CpuResourceMhz:    extraConfigVm.VmSpecSection.CpuResourceMhz,
			MemoryResourceMb:  extraConfigVm.VmSpecSection.MemoryResourceMb,
			MediaSection:      extraConfigVm.VmSpecSection.MediaSection,
			DiskSection:       extraConfigVm.VmSpecSection.DiskSection,
			HardwareVersion:   extraConfigVm.VmSpecSection.HardwareVersion,
			VmToolsVersion:    extraConfigVm.VmSpecSection.VmToolsVersion,
			VirtualCpuType:    extraConfigVm.VmSpecSection.VirtualCpuType,
			TimeSyncWithHost:  extraConfigVm.VmSpecSection.TimeSyncWithHost,
		},
		VirtualHardwareSection: &ExtraConfigVirtualHardwareSectionMarshal{
			NS10:         extraConfigVm.ExtraConfigVirtualHardwareSection.NS10,
			Items:        convertItemsToMarshalItems(extraConfigVm.ExtraConfigVirtualHardwareSection.Items),
			Info:         extraConfigVm.ExtraConfigVirtualHardwareSection.Info,
			ExtraConfigs: newExtraConfig,
		},
		NetworkConnectionSection: &NetworkConnectionSectionMarshal{
			Xmlns:                         extraConfigVm.NetworkConnectionSection.Xmlns,
			OvfRequired:                   extraConfigVm.NetworkConnectionSection.OvfRequired,
			Info:                          extraConfigVm.NetworkConnectionSection.Info,
			HREF:                          extraConfigVm.NetworkConnectionSection.HREF,
			Type:                          extraConfigVm.NetworkConnectionSection.Type,
			PrimaryNetworkConnectionIndex: extraConfigVm.NetworkConnectionSection.PrimaryNetworkConnectionIndex,
			NetworkConnection:             extraConfigVm.NetworkConnectionSection.NetworkConnection,
			Link:                          extraConfigVm.NetworkConnectionSection.Link,
		},
	}
	marshaledXml, err := xml.MarshalIndent(vmMarshal, "", "    ")
	if err != nil {
		return fmt.Errorf("error marshalling vm data: [%v]", err)
	}
	standaloneXmlHeader := strings.Replace(xml.Header, "?>", " standalone=\"yes\"?>", 1)
	reqBody := bytes.NewBufferString(standaloneXmlHeader + string(marshaledXml))
	parsedUrl, err := url.ParseRequestURI(vm.VM.HREF + "/action/reconfigureVm")
	if err != nil {
		return fmt.Errorf("error parsing request uri [%s]: [%v]", vm.VM.HREF+"/action/reconfigureVm", err)
	}
	req := vdc.Client.VCDClient.Client.NewRequest(map[string]string{}, http.MethodPost, *parsedUrl, reqBody)
	req.Header.Add("Content-Type", types.MimeVM)

	// parse response
	resp, err := vdc.Client.VCDClient.Client.Http.Do(req)
	if err != nil {
		return fmt.Errorf("error making request: [%v]", err)
	}
	if resp.StatusCode != http.StatusAccepted {
		respBody, err := ioutil.ReadAll(resp.Body)
		defer resp.Body.Close()

		if err != nil {
			return fmt.Errorf("status code is: [%d], error reading response body: [%v]", resp.StatusCode, err)
		}
		return fmt.Errorf("status code is [%d], response body: [%s]", resp.StatusCode, string(respBody))
	}

	// wait on task to finish
	task, err := vdc.getTaskFromResponse(resp)
	if err != nil {
		return fmt.Errorf("error getting task: [%v]", err)
	}
	if task == nil {
		return fmt.Errorf("nil task returned")
	}
	err = task.WaitTaskCompletion()
	if err != nil {
		return fmt.Errorf("error waiting for task completion after reconfiguring vm: [%v]", err)
	}
	return nil
}

// AddNewMultipleVM will create vmNum VMs in parallel, including recompose VApp of all VMs settings,
// power on VMs and join the cluster with hardcoded script
func (vdc *VdcManager) AddNewMultipleVM(vapp *govcd.VApp, vmNamePrefix string, vmNum int,
	catalogName string, templateName string, placementPolicyName string, computePolicyName string,
	storageProfileName string, guestCustScript string, acceptAllEulas bool, powerOn bool) (govcd.Task, error) {

	klog.V(3).Infof("start adding %d VMs\n", vmNum)

	orgManager, err := NewOrgManager(vdc.Client, vdc.Client.ClusterOrgName)
	if err != nil {
		return govcd.Task{}, fmt.Errorf("error creating orgManager: [%v]", err)
	}

	catalog, err := orgManager.GetCatalogByName(catalogName)
	if err != nil {
		return govcd.Task{}, fmt.Errorf("unable to find catalog [%s] in org [%s]: [%v]",
			catalogName, vdc.OrgName, err)
	}

	vAppTemplateList, err := catalog.QueryVappTemplateList()
	if err != nil {
		return govcd.Task{}, fmt.Errorf("unable to query templates of catalog [%s]: [%v]", catalogName, err)
	}

	var queryVAppTemplate *types.QueryResultVappTemplateType = nil
	for _, template := range vAppTemplateList {
		if template.Name == templateName {
			queryVAppTemplate = template
			break
		}
	}
	if queryVAppTemplate == nil {
		return govcd.Task{}, fmt.Errorf("unable to get template of name [%s] in catalog [%s]",
			templateName, catalogName)
	}

	vAppTemplate := govcd.NewVAppTemplate(&vdc.Client.VCDClient.Client)
	_, err = vdc.Client.VCDClient.Client.ExecuteRequest(queryVAppTemplate.HREF, http.MethodGet,
		"", "error retrieving vApp template: %s", nil, vAppTemplate.VAppTemplate)
	if err != nil {
		return govcd.Task{}, fmt.Errorf("unable to issue get for template with HREF [%s]: [%v]",
			queryVAppTemplate.HREF, err)
	}

	templateHref := vAppTemplate.VAppTemplate.HREF
	if vAppTemplate.VAppTemplate.Children != nil && len(vAppTemplate.VAppTemplate.Children.VM) != 0 {
		templateHref = vAppTemplate.VAppTemplate.Children.VM[0].HREF
	}
	// Status 8 means The object is resolved and powered off.
	// https://vdc-repo.vmware.com/vmwb-repository/dcr-public/94b8bd8d-74ff-4fe3-b7a4-41ae31516ed7/1b42f3b5-8b31-4279-8b3f-547f6c7c5aa8/doc/GUID-843BE3AD-5EF6-4442-B864-BCAE44A51867.html
	if vAppTemplate.VAppTemplate.Status != 8 {
		return govcd.Task{}, fmt.Errorf("vApp Template status [%d] is not ok", vAppTemplate.VAppTemplate.Status)
	}

	var computePolicy *types.ComputePolicy = nil

	if placementPolicyName != "" {
		vmPlacementPolicy, err := orgManager.GetComputePolicyDetailsFromName(placementPolicyName)
		if err != nil {
			return govcd.Task{}, fmt.Errorf("unable to find placement policy [%s]: [%v]", placementPolicyName, err)
		}
		if computePolicy == nil {
			computePolicy = &types.ComputePolicy{}
		}
		computePolicy.VmPlacementPolicy = &types.Reference{
			HREF: vmPlacementPolicy.ID,
		}
	}

	if computePolicyName != "" {
		vmComputePolicy, err := orgManager.GetComputePolicyDetailsFromName(computePolicyName)
		if err != nil {
			return govcd.Task{}, fmt.Errorf("unable to find compute policy [%s]: [%v]", computePolicyName, err)
		}
		if computePolicy == nil {
			computePolicy = &types.ComputePolicy{}
		}
		computePolicy.VmSizingPolicy = &types.Reference{
			HREF: vmComputePolicy.ID,
		}
	}

	var storageProfile *types.Reference = nil

	if storageProfileName != "" {
		storageProfiles := vdc.Client.VDC.Vdc.VdcStorageProfiles.VdcStorageProfile
		for _, profile := range storageProfiles {
			if profile.Name == storageProfileName {
				storageProfile = profile
				break
			}
		}

		if storageProfile == nil {
			return govcd.Task{}, fmt.Errorf("storage profile [%s] chosen to create the VM in vApp [%s] does not exist", storageProfileName, vapp.VApp.Name)
		}
	}

	// for loop to create vms with same settings and append to the sourcedItemList
	sourcedItemList := make([]*types.SourcedCompositionItemParam, vmNum)
	for i := 0; i < vmNum; i++ {
		vmName := vmNamePrefix
		if vmNum != 1 {
			vmName = vmNamePrefix + strconv.Itoa(i)
		}

		sourcedItemList = append(sourcedItemList,
			&types.SourcedCompositionItemParam{
				Source: &types.Reference{
					HREF: templateHref,
					Name: vmName,
				},
				// add this to enable Customization
				VMGeneralParams: &types.VMGeneralParams{
					Name:               vmName,
					Description:        "Auto-created VM",
					NeedsCustomization: true,
					RegenerateBiosUuid: true,
				},
				VAppScopedLocalID: vmName,
				InstantiationParams: &types.InstantiationParams{
					GuestCustomizationSection: &types.GuestCustomizationSection{
						Enabled:               &trueVar,
						AdminPasswordEnabled:  &trueVar,
						AdminPasswordAuto:     &trueVar,
						ResetPasswordRequired: &falseVar,
						ComputerName:          vmName,
					},
					NetworkConnectionSection: &types.NetworkConnectionSection{
						NetworkConnection: []*types.NetworkConnection{
							{
								Network:                 vapp.VApp.NetworkConfigSection.NetworkNames()[0],
								NeedsCustomization:      false,
								IsConnected:             true,
								IPAddressAllocationMode: "POOL",
								NetworkAdapterType:      "VMXNET3",
							},
						},
					},
				},
				StorageProfile: storageProfile,
				ComputePolicy:  computePolicy,
			},
		)
	}

	// vAppComposition consolidates information of VMs which will be sent as ONE request to the VCD API
	vAppComposition := &ComposeVAppWithVMs{
		Ovf:              types.XMLNamespaceOVF,
		Xsi:              types.XMLNamespaceXSI,
		Xmlns:            types.XMLNamespaceVCloud,
		Deploy:           false,
		Name:             vapp.VApp.Name,
		PowerOn:          false,
		Description:      vapp.VApp.Description,
		SourcedItemList:  sourcedItemList,
		AllEULAsAccepted: acceptAllEulas,
	}

	apiEndpoint, _ := url.ParseRequestURI(vapp.VApp.HREF)
	apiEndpoint.Path += "/action/recomposeVApp"

	// execute the task to recomposeVApp
	klog.V(3).Infof("start to compose VApp [%s] with VMs prefix [%s]", vapp.VApp.Name, vmNamePrefix)
	task, err := vdc.Client.VCDClient.Client.ExecuteTaskRequest(apiEndpoint.String(),
		http.MethodPost, types.MimeRecomposeVappParams, "error instantiating a new VM: %s",
		vAppComposition)
	if err != nil {
		return govcd.Task{}, fmt.Errorf("unable to issue call to create VM [%s] in vApp [%s] with template [%s/%s]: [%v]",
			vmNamePrefix, vapp.VApp.Name, catalogName, templateName, err)
	}
	if err = task.WaitTaskCompletion(); err != nil {
		return govcd.Task{}, fmt.Errorf("failed to wait for task [%v] created to add new VM of name [%s]: [%v]",
			task.Task, vmNamePrefix, err)
	}

	vAppRefreshed, err := vdc.Vdc.GetVAppByName(vapp.VApp.Name, true)
	if err != nil {
		return govcd.Task{}, fmt.Errorf("unable to get refreshed vapp by name [%s]: [%v]", vapp.VApp.Name, err)
	}

	if !powerOn {
		return govcd.Task{}, nil
	}

	// once recomposeVApp is done, execute PowerOnAndForceCustomization in go routine to power on VMs in parallel
	// after waitGroup all done, wait 2-3 mins and `kubectl get nodes` in the master to check the nodes

	waitGroup := sync.WaitGroup{}

	vmList := vAppRefreshed.VApp.Children.VM
	klog.V(3).Infof("VApp [%v] has [%v] VMs in total", vAppRefreshed.VApp.Name, len(vmList))
	var vmPowerOnList []*types.Vm
	for i := 0; i < len(vmList); i++ {
		if strings.HasPrefix(vmList[i].Name, vmNamePrefix) {
			vmPowerOnList = append(vmPowerOnList, vmList[i])
		}
	}
	klog.V(3).Infof("VApp [%v] will power on [%v] VMs with prefix [%v], suffix from [%v] to [%v]", vAppRefreshed.VApp.Name, vmNum, vmNamePrefix, 0, len(vmPowerOnList)-1)

	waitGroup.Add(len(vmPowerOnList))

	// TODO: propagate errors here cleanly (create a channel or pass a list of errors and get back a list) or something like `errList := make(error, len(vmPowerOnList))`
	for i := 0; i < len(vmPowerOnList); i++ {
		go func(waitGroup *sync.WaitGroup, i int) {

			defer waitGroup.Done()
			startTime := time.Now()
			klog.V(3).Infof("start powering on vm [%s] at time [%v]\n", vmPowerOnList[i].Name, startTime.Format("2006-01-02 15:04:05"))

			govcdVM, err := vAppRefreshed.GetVMByName(vmPowerOnList[i].Name, false)
			if err != nil {
				klog.V(3).Infof("unable to find vm [%s] in vApp [%s]: [%v]",
					vmList[i].Name, vAppRefreshed.VApp.Name, err)
			}
			for govcdVM == nil {
				klog.V(3).Infof("wait to get vm [%s] in recompose VApp [%s]", vmPowerOnList[i].Name, vapp.VApp.Name)
				govcdVM, err = vAppRefreshed.GetVMByName(vmPowerOnList[i].Name, false)
				if err != nil {
					klog.V(3).Infof("unable to find vm [%s] in vApp [%s]: [%v]",
						vmList[i].Name, vAppRefreshed.VApp.Name, err)
				}
			}

			vmStatus, err := govcdVM.GetStatus()
			if err != nil {
				fmt.Printf("unable to get vm [%s] status before powering on: [%v]", govcdVM.VM.Name, err)
			}
			klog.V(3).Infof("recompose VApp done, vm [%v] status [%v] before powering on, href [%v]", govcdVM.VM.Name, vmStatus, govcdVM.VM.HREF)

			if err := govcdVM.PowerOnAndForceCustomization(); err != nil {
				klog.V(3).Infof("unable to power on and force customization vm [%s]: [%v]", govcdVM.VM.Name, err)
			}

			vmStatus, err = govcdVM.GetStatus()
			if err != nil {
				klog.V(3).Infof("unable to get vm [%s] status after powering on: [%v]", govcdVM.VM.Name, err)
			}
			for vmStatus == "POWERED_OFF" || vmStatus == "PARTIALLY_POWERED_OFF" {
				klog.V(3).Infof("wait powering on vm [%s] current status [%s]", govcdVM.VM.Name, vmStatus)
				vmStatus, err = govcdVM.GetStatus()
				if err != nil {
					klog.V(3).Infof("unable to get vm [%s] status after powering on: [%v]", govcdVM.VM.Name, err)
				}
			}

			endTime := time.Now()
			klog.V(3).Infof("end powering on vm [%s] status [%s] at time [%v]", govcdVM.VM.Name, vmStatus, endTime.Format("2006-01-02 15:04:05"))
			if vmStatus == "POWERED_ON" {
				klog.V(3).Infof("succeed to power on vm [%s] took seconds [%v]", govcdVM.VM.Name, endTime.Sub(startTime).Seconds())
			} else {
				klog.V(3).Infof("fail to power on vm [%s] status [%s]", govcdVM.VM.Name, vmStatus)
			}

		}(&waitGroup, i)

	}
	waitGroup.Wait()

	return govcd.Task{}, nil
}

func (vdc *VdcManager) AddNewVM(vmNamePrefix string, VAppName string, vmNum int,
	catalogName string, templateName string, placementPolicyName string, computePolicyName string,
	storageProfileName string, guestCustScript string, powerOn bool) error {

	if vdc.Vdc == nil {
		return fmt.Errorf("no Vdc created with name [%s]", vdc.VdcName)
	}

	vApp, err := vdc.Vdc.GetVAppByName(VAppName, true)
	if err != nil {
		return fmt.Errorf("unable to get vApp [%s] from Vdc [%s]: [%v]",
			VAppName, vdc.VdcName, err)
	}

	orgManager, err := NewOrgManager(vdc.Client, vdc.Client.ClusterOrgName)
	if err != nil {
		return fmt.Errorf("error creating an orgManager object: [%v]", err)
	}

	catalog, err := orgManager.GetCatalogByName(catalogName)
	if err != nil {
		return fmt.Errorf("unable to find catalog [%s] in org [%s]: [%v]",
			catalogName, vdc.OrgName, err)
	}

	vAppTemplateList, err := catalog.QueryVappTemplateList()
	if err != nil {
		return fmt.Errorf("unable to query templates of catalog [%s]: [%v]", catalogName, err)
	}

	var queryVAppTemplate *types.QueryResultVappTemplateType = nil
	for _, template := range vAppTemplateList {
		if template.Name == templateName {
			queryVAppTemplate = template
			break
		}
	}
	if queryVAppTemplate == nil {
		return fmt.Errorf("unable to get template of name [%s] in catalog [%s]",
			templateName, catalogName)
	}

	vAppTemplate := govcd.NewVAppTemplate(&vdc.Client.VCDClient.Client)
	_, err = vdc.Client.VCDClient.Client.ExecuteRequest(queryVAppTemplate.HREF, http.MethodGet,
		"", "error retrieving vApp template: %s", nil, vAppTemplate.VAppTemplate)
	if err != nil {
		return fmt.Errorf("unable to issue get for template with HREF [%s]: [%v]",
			queryVAppTemplate.HREF, err)
	}

	_, err = vdc.AddNewMultipleVM(vApp, vmNamePrefix, vmNum, catalogName, templateName, placementPolicyName,
		computePolicyName, storageProfileName, guestCustScript, true, powerOn)
	if err != nil {
		return fmt.Errorf(
			"unable to issue call to create VMs with prefix [%s] in vApp [%s] with template [%s/%s]: [%v]",
			vmNamePrefix, VAppName, catalogName, templateName, err)
	}

	return nil
}

func (vdc *VdcManager) DeleteVM(VAppName, vmName string) error {
	vApp, err := vdc.Client.VDC.GetVAppByName(VAppName, true)
	if err != nil {
		return fmt.Errorf("unable to find vApp from name [%s]: [%v]", VAppName, err)
	}

	vm, err := vApp.GetVMByName(vmName, true)
	if err != nil {
		return fmt.Errorf("unable to get vm [%s] in vApp [%s]: [%v]", vmName, VAppName, err)
	}

	if err = vm.Delete(); err != nil {
		return fmt.Errorf("unable to delete vm [%s] in vApp [%s]: [%v]", vmName, VAppName, err)
	}

	return nil
}

func (vdc *VdcManager) GetVAppNameFromVMName(VAppName string, vmName string) (string, error) {
	vm, err := vdc.FindVMByName(VAppName, vmName)
	if err != nil {
		return "", fmt.Errorf("unable to find VM struct from name [%s]: [%v]", vmName, err)
	}

	vApp, err := vm.GetParentVApp()
	if err != nil {
		return "", fmt.Errorf("unable to get vApp for vm with name [%s]: [%v]", vmName, err)
	}

	return vApp.VApp.Name, nil
}

func (vdc *VdcManager) WaitForGuestScriptCompletion(VAppName, vmName string) error {
	vApp, err := vdc.Client.VDC.GetVAppByName(VAppName, true)
	if err != nil {
		return fmt.Errorf("unable to get vApp [%s] from Vdc [%s]: [%v]",
			VAppName, vdc.Client.ClusterOVDCName, err)
	}

	vm, err := vApp.GetVMByName(vmName, false)
	if err != nil {
		return fmt.Errorf("unable to get vm [%s] in vApp [%s]: [%v]", vmName, VAppName, err)
	}
	for {
		status, err := vm.GetGuestCustomizationStatus()
		if err != nil {
			return fmt.Errorf("unable to get guest cust status of vm [%s]: [%v]", vmName, err)
		}
		if status == "GC_COMPLETE" {
			fmt.Printf("Guest Customization complete for vm [%s]. Status = [%s]\n", vmName, status)
			break
		}
		fmt.Printf("Waiting for Guest Customization to complete for vm [%s]. Status = [%s]\n", vmName, status)
		time.Sleep(10 * time.Second)
	}
	return nil
}

func (vdc *VdcManager) RebootVm(vm *govcd.VM) error {
	klog.V(3).Infof("Rebooting VM. [%s]", vm.VM.Name)
	rebootVmUrl, err := url.Parse(fmt.Sprintf("%s/power/action/reboot", vm.VM.HREF))
	if err != nil {
		return fmt.Errorf("failed to parse reboot VM api url for vm [%s]: [%v]", vm.VM.Name, err)
	}
	req := vdc.Client.VCDClient.Client.NewRequest(nil, http.MethodPost, *rebootVmUrl, nil)

	resp, err := vdc.Client.VCDClient.Client.Http.Do(req)
	if err != nil {
		return fmt.Errorf("failed to reboot VM [%s]: [%v]", vm.VM.Name, err)
	}
	vcdTask := govcd.NewTask(&vdc.Client.VCDClient.Client)
	body, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("failed to read response body: [%v]", err)
	}
	if resp.StatusCode != http.StatusAccepted {
		return fmt.Errorf("error while rebooting VM [%s] with status code [%d]: [%s]", vm.VM.Name, resp.StatusCode, body)
	}
	err = xml.Unmarshal(body, &vcdTask.Task)
	if err != nil {
		return fmt.Errorf("failed to unmarshal the task output for reboot vm [%s]: [%v]", vm.VM.Name, err)
	}
	err = vcdTask.WaitTaskCompletion()
	if err != nil {
		return fmt.Errorf("failed to reboot vm [%s]: [%v]", vm.VM.Name, err)
	}
	klog.V(3).Infof("Reboot complete for VM [%s]", vm.VM.Name)
	return nil
}

func (vdc *VdcManager) AddMetadataToVApp(VAppName string, paramMap map[string]string) error {
	vApp, err := vdc.Vdc.GetVAppByName(VAppName, true)
	if err != nil {
		if err == govcd.ErrorEntityNotFound {
			return fmt.Errorf("cannot get the vApp [%s] from Vdc [%s]: [%v]", VAppName, vdc.VdcName, err)
		}
		return fmt.Errorf("error while getting vApp [%s] from Vdc [%s]: [%v]",
			VAppName, vdc.VdcName, err)
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

func (vdc *VdcManager) GetMetadataByKey(vApp *govcd.VApp, key string) (value string, err error) {
	if vApp == nil || vApp.VApp == nil {
		return "", fmt.Errorf("found nil value for vApp")
	}
	metadata, err := vApp.GetMetadata()
	if err != nil {
		return "", fmt.Errorf("unable to get metadata from vApp")
	}
	for _, metadataEntity := range metadata.MetadataEntry {
		if key == metadataEntity.Key {
			return metadataEntity.TypedValue.Value, nil
		}
	}
	return "", fmt.Errorf("metadata record not found for {%s, %s}", key, value)
}
