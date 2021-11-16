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
)

type ExtraConfigVirtualHardwareSection struct {
	XMLName xml.Name `xml:"VirtualHardwareSection"`
	Xmlns   string   `xml:"vcloud,attr,omitempty"`
	NS10    string   `xml:"ns10,attr,omitempty"`

	Info         string                       `xml:"Info"`
	HREF         string                       `xml:"href,attr,omitempty"`
	Type         string                       `xml:"type,attr,omitempty"`
	Items        []*types.VirtualHardwareItem `xml:"Item,omitempty"`
	ExtraConfigs []*ExtraConfig               `xml:"ExtraConfig,omitempty"`
	Links        types.LinkList               `xml:"Link,omitempty"`
}

type ExtraConfigVirtualHardwareSectionMarshal struct {
	NS10         string                `xml:"xmlns:ns10,attr,omitempty"`
	Info         string                `xml:"ovf:Info"`
	ExtraConfigs []*ExtraConfigMarshal `xml:"vmw:ExtraConfig,omitempty"`
}

type ExtraConfig struct {
	Key      string `xml:"key,attr"`
	Value    string `xml:"value,attr"`
	Required bool   `xml:"required,attr"`
}

type ExtraConfigMarshal struct {
	Key      string `xml:"vmw:key,attr"`
	Value    string `xml:"vmw:value,attr"`
	Required bool   `xml:"ovf:required,attr"`
}

type Vm struct {
	Ovf                string `xml:"ovf,attr,omitempty"`
	Xmlns              string `xml:"xmlns,attr,omitempty"`
	Vmw                string `xml:"vmw,attr,omitempty"`
	NeedsCustomization string `xml:"needsCustomization,attr,omitempty"`

	Name        string `xml:"name,attr"`
	Description string `xml:"Description,omitempty"`

	ExtraConfigVirtualHardwareSection *ExtraConfigVirtualHardwareSection `xml:"VirtualHardwareSection,omitempty"`
}

type VmMarshal struct {
	XMLName xml.Name `xml:"Vm"`

	Ovf                string `xml:"xmlns:ovf,attr,omitempty"`
	Xmlns              string `xml:"xmlns,attr,omitempty"`
	Vmw                string `xml:"xmlns:vmw,attr,omitempty"`
	NeedsCustomization string `xml:"needsCustomization,attr,omitempty"`

	Name                   string                                    `xml:"name,attr"`
	VirtualHardwareSection *ExtraConfigVirtualHardwareSectionMarshal `xml:"ovf:VirtualHardwareSection,omitempty"`
}

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
		Ovf:                extraConfigVm.Ovf,
		Xmlns:              extraConfigVm.Xmlns,
		Vmw:                extraConfigVm.Vmw,
		Name:               extraConfigVm.Name,
		NeedsCustomization: extraConfigVm.NeedsCustomization,
		VirtualHardwareSection: &ExtraConfigVirtualHardwareSectionMarshal{
			NS10:         extraConfigVm.ExtraConfigVirtualHardwareSection.NS10,
			Info:         extraConfigVm.ExtraConfigVirtualHardwareSection.Info,
			ExtraConfigs: newExtraConfig,
		},
	}
	marshaledXml, err := xml.MarshalIndent(vmMarshal, "  ", "    ")
	if err != nil {
		return fmt.Errorf("error marshalling vm data: [%v]", err)
	}
	reqBody := bytes.NewBufferString(xml.Header + string(marshaledXml))
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
		return fmt.Errorf("null task returned")
	}
	err = task.WaitTaskCompletion()
	if err != nil {
		return fmt.Errorf("error waiting for task completion after reconfiguring vm: [%v]", err)
	}
	return nil
}
