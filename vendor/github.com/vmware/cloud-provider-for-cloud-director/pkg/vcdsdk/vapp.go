/*
   Copyright 2021 VMware, Inc.
   SPDX-License-Identifier: Apache-2.0
*/

package vcdsdk

import (
	"bytes"
	"encoding/xml"
	"fmt"
	"github.com/vmware/cloud-provider-for-cloud-director/pkg/util"
	"github.com/vmware/go-vcloud-director/v2/govcd"
	"github.com/vmware/go-vcloud-director/v2/types/v56"
	"io/ioutil"
	"k8s.io/klog/v2"
	"net/http"
	"net/url"
	"reflect"
	"strings"
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

func CreateVAppNamePrefix(clusterName string, ovdcID string) (string, error) {
	parts := strings.Split(ovdcID, ":")
	if len(parts) != 4 {
		// urn:vcloud:org:<uuid>
		return "", fmt.Errorf("invalid URN format for OVDC: [%s]", ovdcID)
	}

	return fmt.Sprintf("%s_%s", clusterName, parts[3]), nil
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

	klog.Infof("Trying to find vm [%s] in vApp [%s] by name", vmName, VAppName)
	vApp, err := vdc.Vdc.GetVAppByName(VAppName, true)
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

	vApp, err := vdc.Vdc.GetVAppByName(VAppName, true)
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

func (vdc *VdcManager) SetMultiVmExtraConfigKeyValuePairs(vm *govcd.VM, extraConfigMap map[string]string,
	required bool) (govcd.Task, error) {
	_, extraConfigVm, err := vdc.getVmExtraConfigs(vm)
	if err != nil {
		return govcd.Task{}, fmt.Errorf("error retrieving vm extra configs: [%v]", err)
	}

	newExtraConfig := make([]*ExtraConfigMarshal, len(extraConfigMap))
	idx := 0
	for key, val := range extraConfigMap {
		newExtraConfig[idx] = &ExtraConfigMarshal{
			Key:      key,
			Value:    val,
			Required: required,
		}
		idx = idx + 1
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
		return govcd.Task{}, fmt.Errorf("error marshalling vm data: [%v]", err)
	}
	standaloneXmlHeader := strings.Replace(xml.Header, "?>", " standalone=\"yes\"?>", 1)
	reqBody := bytes.NewBufferString(standaloneXmlHeader + string(marshaledXml))
	parsedUrl, err := url.ParseRequestURI(vm.VM.HREF + "/action/reconfigureVm")
	if err != nil {
		return govcd.Task{}, fmt.Errorf("error parsing request uri [%s]: [%v]", vm.VM.HREF+"/action/reconfigureVm", err)
	}
	req := vdc.Client.VCDClient.Client.NewRequest(map[string]string{}, http.MethodPost, *parsedUrl, reqBody)
	req.Header.Add("Content-Type", types.MimeVM)

	// parse response
	resp, err := vdc.Client.VCDClient.Client.Http.Do(req)
	if err != nil {
		return govcd.Task{}, fmt.Errorf("error making request: [%v]", err)
	}
	if resp.StatusCode != http.StatusAccepted {
		respBody, err := ioutil.ReadAll(resp.Body)
		defer resp.Body.Close()

		if err != nil {
			return govcd.Task{}, fmt.Errorf("status code is: [%d], error reading response body: [%v]", resp.StatusCode, err)
		}
		return govcd.Task{}, fmt.Errorf("status code is [%d], response body: [%s]", resp.StatusCode, string(respBody))
	}

	// wait on task to finish
	task, err := vdc.getTaskFromResponse(resp)
	if err != nil {
		return govcd.Task{}, fmt.Errorf("error getting task: [%v]", err)
	}
	if task == nil {
		return govcd.Task{}, fmt.Errorf("nil task returned")
	}

	return *task, nil
}

func (vdc *VdcManager) AddNewTkgVM(vmNamePrefix string, VAppName string, catalogName string, templateName string,
	placementPolicyName string, computePolicyName string, storageProfileName string) (govcd.Task, error) {

	// In TKG >= 1.6.0, there is a missing file at /etc/cloud/cloud.cfg.d/
	// that tells cloud-init the datasource. Without this file, a file prefixed with 90-*
	// lists possible sources and causes cloud-init to read from OVF.
	// This file is present in TKG < 1.6.0 and lists the datasource as "VMwareGuestInfo".
	// In TKG < 1.6.0, this file has the 99-* prefix (the higher the value, the higher the priority)
	// For TKG >= 1.6.0, this datasource name is "VMware". In order for this added file not to conflict
	// with the datasource file in TKG < 1.6.0, we prefix the file with 98-*, and we specify the datasource
	// as "VMware". We use guest customization to add this file.
	guestCustScript := `
#!/usr/bin/env bash
cat > /etc/cloud/cloud.cfg.d/98-cse-vmware-datasource.cfg <<EOF
datasource_list: [ "VMware" ]
EOF
`

	task, err := vdc.AddNewVM(vmNamePrefix, VAppName, catalogName, templateName, placementPolicyName,
		computePolicyName, storageProfileName, guestCustScript)
	if err != nil {
		return govcd.Task{}, fmt.Errorf("error for adding TKG VM to vApp[%s]: [%v]", VAppName, err)
	}
	return task, nil
}

func (vdc *VdcManager) getTemplateHREFFromName(orgManager *OrgManager,
	catalogName string, templateName string) (string, error) {

	catalog, err := orgManager.GetCatalogByName(catalogName)
	if err != nil {
		return "", fmt.Errorf("unable to find catalog [%s] in org [%s]: [%v]",
			catalogName, vdc.OrgName, err)
	}

	vAppTemplateList, err := catalog.QueryVappTemplateList()
	if err != nil {
		return "", fmt.Errorf("unable to query templates of catalog [%s]: [%v]", catalogName, err)
	}

	var queryVAppTemplate *types.QueryResultVappTemplateType = nil
	for _, template := range vAppTemplateList {
		if template.Name == templateName {
			queryVAppTemplate = template
			break
		}
	}
	if queryVAppTemplate == nil {
		return "", fmt.Errorf("unable to get template of name [%s] in catalog [%s]",
			templateName, catalogName)
	}
	vAppTemplate := govcd.NewVAppTemplate(&vdc.Client.VCDClient.Client)
	resp, err := vdc.Client.VCDClient.Client.ExecuteRequest(queryVAppTemplate.HREF, http.MethodGet,
		"", "error retrieving vApp template: %s", nil, vAppTemplate.VAppTemplate)
	if err != nil {
		return "", fmt.Errorf("unable to issue get for template with HREF [%s]: [%v], resp = [%v]",
			queryVAppTemplate.HREF, err, resp)
	}
	templateHref := vAppTemplate.VAppTemplate.HREF
	if vAppTemplate.VAppTemplate.Children != nil && len(vAppTemplate.VAppTemplate.Children.VM) != 0 {
		templateHref = vAppTemplate.VAppTemplate.Children.VM[0].HREF
	}

	return templateHref, nil
}

func (vdc *VdcManager) getComputePolicy(orgManager *OrgManager, computePolicyName string, placementPolicyName string) (*types.ComputePolicy, error) {
	var computePolicy *types.ComputePolicy = nil
	if placementPolicyName != "" {
		vmPlacementPolicy, err := orgManager.GetComputePolicyDetailsFromName(placementPolicyName)
		if err != nil {
			return nil, fmt.Errorf("unable to find placement policy [%s]: [%v]", placementPolicyName, err)
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
			return nil, fmt.Errorf("unable to find compute policy [%s]: [%v]", computePolicyName, err)
		}
		if computePolicy == nil {
			computePolicy = &types.ComputePolicy{}
		}
		computePolicy.VmSizingPolicy = &types.Reference{
			HREF: vmComputePolicy.ID,
		}
	}

	return computePolicy, nil
}

func (vdc *VdcManager) getStorageProfile(storageProfileName string) (*types.Reference, error) {
	if storageProfileName == "" {
		return nil, nil
	}

	for _, profile := range vdc.Vdc.Vdc.VdcStorageProfiles.VdcStorageProfile {
		if profile.Name == storageProfileName {
			return profile, nil
		}
	}

	return nil, fmt.Errorf("storage profile [%s] could not be found", storageProfileName)
}

func (vdc *VdcManager) AddNewVM(vmName string, VAppName string, catalogName string, templateName string,
	placementPolicyName string, computePolicyName string, storageProfileName string, guestCustScript string) (govcd.Task, error) {

	if vdc.Vdc == nil {
		return govcd.Task{}, fmt.Errorf("no Vdc created with name [%s]", vdc.VdcName)
	}

	vApp, err := vdc.Vdc.GetVAppByName(VAppName, true)
	if err != nil {
		return govcd.Task{}, fmt.Errorf("unable to get vApp [%s] from Vdc [%s]: [%v]",
			VAppName, vdc.VdcName, err)
	}

	orgManager, err := NewOrgManager(vdc.Client, vdc.Client.ClusterOrgName)
	if err != nil {
		return govcd.Task{}, fmt.Errorf("error creating an orgManager object: [%v]", err)
	}

	templateHREF, err := vdc.getTemplateHREFFromName(orgManager, catalogName, templateName)
	if err != nil {
		return govcd.Task{}, fmt.Errorf("unable to get catalog/template [%s/%s] in org [%s]: [%v]",
			catalogName, templateName, vdc.OrgName, err)
	}

	computePolicy, err := vdc.getComputePolicy(orgManager, computePolicyName, placementPolicyName)
	if err != nil {
		return govcd.Task{}, fmt.Errorf("unable to get policies from computePolicy [%s], placementPolicy [%s] in org [%s]: [%v]",
			computePolicyName, placementPolicyName, vdc.OrgName, err)
	}

	storageProfile, err := vdc.getStorageProfile(storageProfileName)
	if err != nil {
		return govcd.Task{}, fmt.Errorf("unable to find storage Profile [%s] in ovdc [%s]: [%v]", storageProfileName,
			vdc.VdcName, err)
	}

	// these are the settings used in VMware Cloud Director
	passwd, err := util.GeneratePassword(15, 5, 3, false, false)
	if err != nil {
		return govcd.Task{}, fmt.Errorf("failed to generate a password to create a VM in the VApp [%s]", vApp.VApp.Name)
	}

	vmDef := &types.ReComposeVAppParams{
		Ovf:                 types.XMLNamespaceOVF,
		Xsi:                 types.XMLNamespaceXSI,
		Xmlns:               types.XMLNamespaceVCloud,
		Name:                vApp.VApp.Name,
		Deploy:              false,
		PowerOn:             false,
		LinkedClone:         false,
		Description:         vApp.VApp.Description,
		VAppParent:          nil,
		InstantiationParams: nil,
		SourcedItem: &types.SourcedCompositionItemParam{
			Source: &types.Reference{
				HREF: templateHREF,
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
					AdminPasswordAuto:     &falseVar,
					AdminPassword:         passwd,
					ResetPasswordRequired: &falseVar,
					ComputerName:          vmName,
					CustomizationScript:   guestCustScript,
				},
				NetworkConnectionSection: &types.NetworkConnectionSection{
					NetworkConnection: []*types.NetworkConnection{
						{
							Network:                 vApp.VApp.NetworkConfigSection.NetworkNames()[0],
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
		AllEULAsAccepted: true,
		DeleteItem:       nil,
	}

	apiEndpoint, _ := url.ParseRequestURI(vApp.VApp.HREF)
	apiEndpoint.Path += "/action/recomposeVApp"

	// execute the task to recomposeVApp
	klog.V(3).Infof("START to compose VApp [%s] with VMs prefix [%s]", vApp.VApp.Name, vmName)
	task, err := vdc.Client.VCDClient.Client.ExecuteTaskRequest(apiEndpoint.String(),
		http.MethodPost, types.MimeRecomposeVappParams, "error instantiating a new VM: [%s]",
		vmDef)
	if err != nil {
		return govcd.Task{}, fmt.Errorf("unable to issue call to create VM [%s] in vApp [%s] with template [%s/%s]: [%v]",
			vmName, vApp.VApp.Name, catalogName, templateName, err)
	}

	return task, nil
}

func (vdc *VdcManager) DeleteVM(VAppName, vmName string) (govcd.Task, error) {
	vApp, err := vdc.Vdc.GetVAppByName(VAppName, true)
	if err != nil {
		return govcd.Task{}, fmt.Errorf("unable to find vApp from name [%s]: [%v]", VAppName, err)
	}

	vm, err := vApp.GetVMByName(vmName, true)
	if err != nil {
		return govcd.Task{}, fmt.Errorf("unable to get vm [%s] in vApp [%s]: [%v]", vmName, VAppName, err)
	}

	task, err := vm.DeleteAsync()
	if err != nil {
		return govcd.Task{}, fmt.Errorf("unable to delete vm [%s] in vApp [%s]: [%v]", vmName, VAppName, err)
	}

	return task, nil
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
	vApp, err := vdc.Vdc.GetVAppByName(VAppName, true)
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
