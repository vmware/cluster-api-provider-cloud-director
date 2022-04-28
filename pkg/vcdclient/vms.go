/*
   Copyright 2021 VMware, Inc.
   SPDX-License-Identifier: Apache-2.0
*/

package vcdclient

import (
	"encoding/xml"
	"fmt"
	"github.com/vmware/cluster-api-provider-cloud-director/pkg/vcdtypes"
	"io/ioutil"
	"k8s.io/klog"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/vmware/go-vcloud-director/v2/govcd"
	"github.com/vmware/go-vcloud-director/v2/types/v56"
)

const (
	// VCDVMIDPrefix is a prefix added to VM objects by VCD. This needs
	// to be removed for query operations.
	VCDVMIDPrefix = "urn:vcloud:vm:"
)

func (vdc *VdcManager) FindVMByName(vmName string) (*govcd.VM, error) {
	if vmName == "" {
		return nil, fmt.Errorf("vmName mandatory for FindVMByName")
	}

	// Query is be delimited to org where user exists. The expectation is that
	// there will be exactly one VM with that name.
	results, err := vdc.Vdc.QueryWithNotEncodedParams(
		map[string]string{
			"type":   "vm",
			"format": "records",
		},
		map[string]string{
			"filter": fmt.Sprintf("(name==%s)", vmName),
		},
	)
	if err != nil {
		return nil, fmt.Errorf("unable to query for VM [%s]: [%v]", vmName, err)
	}

	if results.Results.Total > 1 {
		return nil, fmt.Errorf("obtained [%d] VMs for name [%s], expected only one",
			int(results.Results.Total), vmName)
	}

	if results.Results.Total == 0 {
		return nil, govcd.ErrorEntityNotFound
	}

	href := results.Results.VMRecord[0].HREF
	vm, err := vdc.Client.VcdClient.Client.GetVMByHref(href)
	if err != nil {
		return nil, fmt.Errorf("unable to find vm by HREF [%s]: [%v]", href, err)
	}

	return vm, nil
}

func (vdc *VdcManager) FindVMByUUID(vcdVmUUID string) (*govcd.VM, error) {
	if vcdVmUUID == "" {
		return nil, fmt.Errorf("vmUUID mandatory for FindVMByUUID")
	}

	vmUUID := strings.TrimPrefix(vcdVmUUID, VCDVMIDPrefix)
	href := fmt.Sprintf("%v/vApp/vm-%s", vdc.Client.VcdClient.Client.VCDHREF.String(),
		strings.ToLower(vmUUID))
	vm, err := vdc.Client.VcdClient.Client.GetVMByHref(href)
	if err != nil {
		return nil, fmt.Errorf("unable to find vm by HREF [%s]: [%v]", href, err)
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

// AddNewMultipleVM will create vmNum VMs in parallel, including recompose VApp of all VMs settings,
// power on VMs and join the cluster with hardcoded script
func (vdc *VdcManager) AddNewMultipleVM(vapp *govcd.VApp, vmNamePrefix string, vmNum int,
	catalogName string, templateName string, placementPolicyName string, computePolicyName string,
	storageProfileName string, guestCustScript string, acceptAllEulas bool, powerOn bool) (govcd.Task, error) {

	klog.V(3).Infof("start adding %d VMs\n", vmNum)

	catalog, err := vdc.Client.GetCatalogByName(vdc.OrgName, catalogName)
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

	vAppTemplate := govcd.NewVAppTemplate(&vdc.Client.VcdClient.Client)
	_, err = vdc.Client.VcdClient.Client.ExecuteRequest(queryVAppTemplate.HREF, http.MethodGet,
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
		vmPlacementPolicy, err := vdc.Client.GetComputePolicyDetailsFromName(placementPolicyName)
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
		vmComputePolicy, err := vdc.Client.GetComputePolicyDetailsFromName(computePolicyName)
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
		storageProfiles := vdc.Client.Vdc.Vdc.VdcStorageProfiles.VdcStorageProfile
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
	vAppComposition := &vcdtypes.ComposeVAppWithVMs{
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
	task, err := vdc.Client.VcdClient.Client.ExecuteTaskRequest(apiEndpoint.String(),
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

func (vdc *VdcManager) AddNewVM(vmNamePrefix string, vAppName string, vmNum int,
	catalogName string, templateName string, placementPolicyName string, computePolicyName string,
	storageProfileName string, guestCustScript string, powerOn bool) error {

	if vdc.Vdc == nil {
		return fmt.Errorf("no Vdc created with name [%s]", vdc.VdcName)
	}

	vApp, err := vdc.Vdc.GetVAppByName(vAppName, true)
	if err != nil {
		return fmt.Errorf("unable to get vApp [%s] from Vdc [%s]: [%v]",
			vAppName, vdc.VdcName, err)
	}

	catalog, err := vdc.Client.GetCatalogByName(vdc.OrgName, catalogName)
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

	vAppTemplate := govcd.NewVAppTemplate(&vdc.Client.VcdClient.Client)
	_, err = vdc.Client.VcdClient.Client.ExecuteRequest(queryVAppTemplate.HREF, http.MethodGet,
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
			vmNamePrefix, vAppName, catalogName, templateName, err)
	}

	return nil
}

func (vdc *VdcManager) DeleteVM(vAppName string, vmName string) error {
	vApp, err := vdc.Vdc.GetVAppByName(vAppName, true)
	if err != nil {
		return fmt.Errorf("unable to find vApp from name [%s]: [%v]", vAppName, err)
	}

	vm, err := vApp.GetVMByName(vmName, true)
	if err != nil {
		return fmt.Errorf("unable to get vm [%s] in vApp [%s]: [%v]", vmName, vAppName, err)
	}

	if err = vm.Delete(); err != nil {
		return fmt.Errorf("unable to delete vm [%s] in vApp [%s]: [%v]", vmName, vAppName, err)
	}

	return nil
}

func (vdc *VdcManager) GetVAppNameFromVMName(vmName string) (string, error) {
	vm, err := vdc.FindVMByName(vmName)
	if err != nil {
		return "", fmt.Errorf("unable to find VM struct from name [%s]: [%v]", vmName, err)
	}

	vApp, err := vm.GetParentVApp()
	if err != nil {
		return "", fmt.Errorf("unable to get vApp for vm with name [%s]: [%v]", vmName, err)
	}

	return vApp.VApp.Name, nil
}

func (vdc *VdcManager) WaitForGuestScriptCompletion(vmName string, vAppName string) error {
	vApp, err := vdc.Vdc.GetVAppByName(vAppName, true)
	if err != nil {
		return fmt.Errorf("unable to get vApp [%s] from Vdc [%s]: [%v]",
			vAppName, vdc.VdcName, err)
	}

	vm, err := vApp.GetVMByName(vmName, false)
	if err != nil {
		return fmt.Errorf("unable to get vm [%s] in vApp [%s]: [%v]", vmName, vAppName, err)
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

func (client *Client) RebootVm(vm *govcd.VM) error {
	klog.V(3).Infof("Rebooting VM. [%s]", vm.VM.Name)
	rebootVmUrl, err := url.Parse(fmt.Sprintf("%s/power/action/reboot", vm.VM.HREF))
	if err != nil {
		return fmt.Errorf("failed to parse reboot VM api url for vm [%s]: [%v]", vm.VM.Name, err)
	}
	req := client.VcdClient.Client.NewRequest(nil, http.MethodPost, *rebootVmUrl, nil)

	resp, err := client.VcdClient.Client.Http.Do(req)
	if err != nil {
		return fmt.Errorf("failed to reboot VM [%s]: [%v]", vm.VM.Name, err)
	}
	vcdTask := govcd.NewTask(&client.VcdClient.Client)
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
