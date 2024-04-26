package vcdsdk

import (
	"fmt"
	"github.com/go-openapi/errors"
	"github.com/vmware/go-vcloud-director/v2/govcd"
	"github.com/vmware/go-vcloud-director/v2/types/v56"
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
	isMultiZoneCluster bool) (*govcd.VM, string, error) {

	org, err := orgManager.Client.VCDClient.GetOrgByName(orgManager.OrgName)
	if err != nil {
		return nil, "", fmt.Errorf("unable to get org by name [%s]: [%v]", orgManager.OrgName, err)
	}

	var vmRecordList []*types.QueryResultVMRecordType = nil

	if vmName != "" {
		vmRecordList, err = govcd.QueryVmList(types.VmQueryFilterOnlyDeployed, &orgManager.Client.VCDClient.Client,
			map[string]string{"name": vmName})
		if err != nil {
			return nil, "", fmt.Errorf("unable to query all VMs using name [%s]: [%v]", vmName, err)
		}
	} else if vmId != "" {
		vmRecordList, err = govcd.QueryVmList(types.VmQueryFilterOnlyDeployed, &orgManager.Client.VCDClient.Client,
			map[string]string{"id": vmId})
		if err != nil {
			return nil, "", fmt.Errorf("unable to query all VMs using ID [%s]: [%v]", vmId, err)
		}
	} else {
		return nil, "", fmt.Errorf("unable to query VM when name and ID are both not provided")
	}

	for _, vmRecord := range vmRecordList {
		if vmId != "" || // there is no need to correlate VM ID since it is unique across VCD
			(vmName != "" && vmRecord.Name == vmName) {
			vdc, err := org.GetVDCByHref(vmRecord.VdcHREF)
			if err != nil {
				return nil, "", fmt.Errorf("found vm [%s, %s] in VDC with HREF[%s], but VDC could not be queried: [%v]",
					vmName, vmId, vmRecord.VdcHREF, err)
			}

			vm, err := orgManager.Client.VCDClient.Client.GetVMByHref(vmRecord.HREF)
			if err != nil {
				return nil, "", fmt.Errorf("unable to find VM [%s, %s] by HREF [%s]: [%v]",
					vmName, vmId, vmRecord.HREF, err)
			}

			vApp, err := vm.GetParentVApp()
			if err != nil {
				return nil, "", fmt.Errorf("unable to get parent of VM [%s, %s]: [%v]", vmName, vmId, err)
			}

			if isMultiZoneCluster {
				vdcUUID := vdc.Vdc.ID
				vdcIdParts := strings.Split(vdc.Vdc.ID, ":")
				if len(vdcIdParts) == 4 {
					vdcUUID = vdcIdParts[3]
				}
				// In this case we need to check if the vApp Name is a proper prefix
				if !strings.HasPrefix(vApp.VApp.Name, fmt.Sprintf("%s_%s", clusterName, vdcUUID)) {
					continue
				}
			} else {
				if vApp.VApp.Name != clusterName {
					continue
				}
			}

			return vm, vdc.Vdc.Name, nil
		}
	}

	return nil, "", govcd.ErrorEntityNotFound
}
