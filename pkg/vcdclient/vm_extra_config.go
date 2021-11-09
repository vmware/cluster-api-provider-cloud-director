package vcdclient

import (
	"bytes"
	"encoding/xml"
	"fmt"
	"github.com/vmware/go-vcloud-director/v2/govcd"
	"github.com/vmware/go-vcloud-director/v2/types/v56"
	"io/ioutil"
	"net/http"
	"net/url"
	"reflect"
	"strings"
)

// the returned extra configs is part of the returned vm
func (client *Client) getVmExtraConfigs(vm *govcd.VM) ([]*ExtraConfig, *Vm, error) {
	extraConfigVm := &Vm{}

	if vm.VM.HREF == "" {
		return nil, nil, fmt.Errorf("cannot refresh, invalid reference url")
	}

	_, err := client.VcdClient.Client.ExecuteRequest(vm.VM.HREF, http.MethodGet,
		"", "error retrieving virtual hardware: %s", nil, extraConfigVm)
	if err != nil {
		return nil, nil, fmt.Errorf("error executing GET request for vm: [%v]", err)
	}

	return extraConfigVm.ExtraConfigVirtualHardwareSection.ExtraConfigs, extraConfigVm, nil
}

func (client *Client) GetExtraConfigValue(vm *govcd.VM, key string) (string, error) {
	extraConfigs, _, err := client.getVmExtraConfigs(vm)
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

func (client *Client) getTaskFromResponse(resp *http.Response) (*govcd.Task, error) {
	task := govcd.NewTask(&client.VcdClient.Client)
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

func (client *Client) SetVmExtraConfigKeyValue(vm *govcd.VM, key string, value string, required bool) error {
	_, extraConfigVm, err := client.getVmExtraConfigs(vm)
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
	url, err := url.ParseRequestURI(vm.VM.HREF + "/action/reconfigureVm")
	if err != nil {
		return fmt.Errorf("error parsing request uri [%s]: [%v]", vm.VM.HREF+"/action/reconfigureVm", err)
	}
	req := client.VcdClient.Client.NewRequest(map[string]string{}, http.MethodPost, *url, reqBody)
	req.Header.Add("Content-Type", types.MimeVM)

	// parse response
	resp, err := client.VcdClient.Client.Http.Do(req)
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
	task, err := client.getTaskFromResponse(resp)
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
