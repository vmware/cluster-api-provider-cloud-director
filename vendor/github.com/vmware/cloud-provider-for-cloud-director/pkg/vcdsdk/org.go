package vcdsdk

import (
	"fmt"
	"github.com/go-openapi/errors"
	"github.com/vmware/go-vcloud-director/v2/govcd"
	"github.com/vmware/go-vcloud-director/v2/types/v56"
	"net/url"
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
