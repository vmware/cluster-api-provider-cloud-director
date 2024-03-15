package vcdsdk

import (
	"fmt"
	"github.com/go-openapi/errors"
	"github.com/vmware/go-vcloud-director/v2/govcd"
	"github.com/vmware/go-vcloud-director/v2/types/v56"
	"k8s.io/klog/v2"
	"net/url"
	"strings"
)

type OrgManager struct {
	// client should be refreshed separately
	Client  *Client
	OrgName string
}

func NewOrgManager(client *Client, orgName string) (*OrgManager, error) {
	// if orgName is empty, use the clusteOrgName from the client as the orgName
	if orgName == "" && client.ClusterOrgName == "" {
		if client.ClusterOrgName == "" {
			return nil, fmt.Errorf("could not find a valid OrgName to create orgManager")
		}
		orgName = client.ClusterOrgName
	}
	return &OrgManager{
		Client:  client,
		OrgName: orgName,
	}, nil
}

func (orgManager *OrgManager) GetCatalogByName(catalogName string) (*govcd.Catalog, error) {
	org, err := orgManager.Client.VCDClient.GetOrgByName(orgManager.OrgName)
	if err != nil {
		return nil, fmt.Errorf("unable to get vcd organization [%s]: [%v]", orgManager.OrgName, err)
	}
	if err := org.Refresh(); err != nil {
		return nil, fmt.Errorf("unable to refresh org [%s]: [%v]", orgManager.OrgName, err)
	}
	catalog, err := org.GetCatalogByName(catalogName, true)
	if err != nil {
		return catalog, fmt.Errorf("unable to find catalog [%s] in org [%s]", catalogName, orgManager.OrgName)
	}
	return catalog, nil
}

func (orgManager *OrgManager) GetComputePolicyDetailsFromName(computePolicyName string) (*types.VdcComputePolicy, error) {
	org, err := orgManager.Client.VCDClient.GetOrgByName(orgManager.OrgName)
	if err != nil {
		return nil, fmt.Errorf("unable to get org [%s] by name: [%v]", orgManager.OrgName, err)
	}

	vdcComputePolicies, err := org.GetAllVdcComputePolicies(url.Values{})
	if err != nil {
		return nil, fmt.Errorf("unable to get all compute policies for [%s] by name: [%v]",
			orgManager.OrgName, err)
	}

	var computePolicy *types.VdcComputePolicy = nil
	for _, vdcComputePolicy := range vdcComputePolicies {
		if vdcComputePolicy.VdcComputePolicy == nil {
			continue
		}
		if vdcComputePolicy.VdcComputePolicy.Name == computePolicyName {
			computePolicy = vdcComputePolicy.VdcComputePolicy
			break
		}
	}

	if computePolicy == nil {
		return nil, errors.NotFound("unable to find compute policy [%s]", computePolicyName)
	}

	return computePolicy, nil
}

func (orgManager *OrgManager) SearchVMAcrossVDCs(vmName string, clusterName string, vmId string,
	ovdcNameList []string) (*govcd.VM, string, error) {

	org, err := orgManager.Client.VCDClient.GetOrgByName(orgManager.OrgName)
	if err != nil {
		return nil, "", fmt.Errorf("unable to get org by name [%s]: [%v]", orgManager.OrgName, err)
	}

	for _, ovdcName := range ovdcNameList {
		klog.Infof("Looking for VM [name:%s],[id:%s] of cluster [%s] in OVDC [%s]",
			vmName, vmId, clusterName, ovdcName)

		vdc, err := org.GetVDCByName(ovdcName, true)
		if err != nil {
			klog.Infof("unable to query VDC [%s] in Org [%s] by name: [%v]",
				ovdcName, orgManager.OrgName, err)
			continue
		}

		vAppNamePrefix, err := CreateVAppNamePrefix(clusterName, vdc.Vdc.ID)
		if err != nil {
			klog.Infof("Unable to create a vApp name prefix for cluster [%s] in OVDC [%s] with OVDC ID [%s]: [%v]",
				clusterName, vdc.Vdc.Name, vdc.Vdc.ID, err)
			continue
		}

		klog.Infof("Looking for vApps with a prefix of [%s]", vAppNamePrefix)
		vAppList := vdc.GetVappList()
		// check if the VM exists in any cluster-vApps in this OVDC
		for _, vApp := range vAppList {
			if strings.HasPrefix(vApp.Name, vAppNamePrefix) {
				// check if VM exists
				klog.Infof("Looking for VM [name:%s],[id:%s] in vApp [%s] in OVDC [%s] is a vApp in cluster [%s]",
					vmName, vmId, vApp.Name, vdc.Vdc.Name, clusterName)
				vdcManager, err := NewVDCManager(orgManager.Client, orgManager.OrgName, vdc.Vdc.Name)
				if err != nil {
					return nil, "", fmt.Errorf("error creating VDCManager object for VDC [%s]: [%v]",
						vdc.Vdc.Name, err)
				}

				var vm *govcd.VM = nil
				if vmName != "" {
					vm, err = vdcManager.FindVMByName(vApp.Name, vmName)
				} else if vmId != "" {
					vm, err = vdcManager.FindVMByUUID(vApp.Name, vmId)
				} else {
					return nil, "", fmt.Errorf("either vm name [%s] or ID [%s] should be passed", vmName, vmId)
				}
				if err != nil {
					klog.Infof("Could not find VM [name:%s],[id:%s] in vApp [%s] of Cluster [%s] in OVDC [%s]: [%v]",
						vmName, vmId, vApp.Name, clusterName, vdc.Vdc.Name, err)
					continue
				}

				// If we reach here, we found the VM
				klog.Infof("Found VM [name:%s],[id:%s] in vApp [%s] of Cluster [%s] in OVDC [%s]: [%v]",
					vmName, vmId, vApp.Name, clusterName, vdc.Vdc.Name, err)
				return vm, vdc.Vdc.Name, nil
			}
		}

		klog.Infof("Could not find VM [name:%s],[id:%s] of cluster [%s] in OVDC [%s]",
			vmName, vmId, clusterName, ovdcName)
	}

	return nil, "", govcd.ErrorEntityNotFound
}
