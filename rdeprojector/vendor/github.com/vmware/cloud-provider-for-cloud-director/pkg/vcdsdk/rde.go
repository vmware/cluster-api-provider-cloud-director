package vcdsdk

import (
	"context"
	"fmt"
	"github.com/vmware/cloud-provider-for-cloud-director/pkg/util"
	swaggerClient "github.com/vmware/cloud-provider-for-cloud-director/pkg/vcdswaggerclient"
	"k8s.io/klog"
	"net/http"
	"strings"
)

const (
	NoRdePrefix = `NO_RDE_`
)

type RDEManager struct {
	ClusterID string
	// Client will be refreshed separately
	Client *Client
}

func NewRDEManager(client *Client, clusterID string) *RDEManager {
	return &RDEManager{
		ClusterID: clusterID,
		Client:    client,
	}
}

func (rm *RDEManager) GetRDEVirtualIps(ctx context.Context) ([]string, string, *swaggerClient.DefinedEntity, error) {
	if rm.ClusterID == "" || strings.HasPrefix(rm.ClusterID, NoRdePrefix) {
		klog.Infof("ClusterID [%s] is empty or generated", rm.ClusterID)
		return nil, "", nil, nil
	}

	client := rm.Client
	defEnt, _, etag, err := client.APIClient.DefinedEntityApi.GetDefinedEntity(ctx, rm.ClusterID)
	if err != nil {
		return nil, "", nil, fmt.Errorf("error when getting defined entity: [%v]", err)
	}

	virtualIpStrs, err := util.GetVirtualIPsFromRDE(&defEnt)
	if err != nil {
		return nil, "", nil, fmt.Errorf("failed to retrieve Virtual IPs from RDE [%s]: [%v]",
			rm.ClusterID, err)
	}
	return virtualIpStrs, etag, &defEnt, nil
}

// This function will modify the passed in defEnt
func (rm *RDEManager) updateRDEVirtualIps(ctx context.Context, updatedIps []string, etag string,
	defEnt *swaggerClient.DefinedEntity) (*http.Response, error) {
	defEnt, err := util.ReplaceVirtualIPsInRDE(defEnt, updatedIps)
	if err != nil {
		return nil, fmt.Errorf("failed to locally edit RDE with ID [%s] with virtual IPs: [%v]", rm.ClusterID, err)
	}
	client := rm.Client
	// can pass invokeHooks
	_, httpResponse, err := client.APIClient.DefinedEntityApi.UpdateDefinedEntity(ctx, *defEnt, etag, rm.ClusterID, nil)
	if err != nil {
		return httpResponse, fmt.Errorf("error when updating defined entity [%s]: [%v]", rm.ClusterID, err)
	}
	return httpResponse, nil
}

func (rm *RDEManager) addVirtualIpToRDE(ctx context.Context, addIp string) error {
	if addIp == "" {
		klog.Infof("VIP is empty, hence not adding anything to RDE")
		return nil
	}
	if rm.ClusterID == "" || strings.HasPrefix(rm.ClusterID, NoRdePrefix) {
		klog.Infof("ClusterID [%s] is empty or generated, hence not adding VIP [%s] from RDE",
			rm.ClusterID, addIp)
		return nil
	}

	numRetries := 10
	for i := 0; i < numRetries; i++ {
		currIps, etag, defEnt, err := rm.GetRDEVirtualIps(ctx)
		if err != nil {
			return fmt.Errorf("error getting current vips: [%v]", err)
		}

		// check if need to update RDE
		foundAddIp := false
		for _, ip := range currIps {
			if ip == addIp {
				foundAddIp = true
				break
			}
		}
		if foundAddIp {
			return nil // no need to update RDE
		}

		updatedIps := append(currIps, addIp)
		httpResponse, err := rm.updateRDEVirtualIps(ctx, updatedIps, etag, defEnt)
		if err != nil {
			if httpResponse.StatusCode == http.StatusPreconditionFailed {
				klog.Infof("Wrong ETag while adding virtual IP [%s]", addIp)
				continue
			}
			return fmt.Errorf("error when adding virtual ip [%s] to RDE: [%v]", addIp, err)
		}
		klog.Infof("Successfully updated RDE [%s] with virtual IP [%s]", rm.ClusterID, addIp)
		return nil
	}

	return fmt.Errorf("unable to update rde due to incorrect etag after [%d]] tries", numRetries)
}

func (rm *RDEManager) removeVirtualIpFromRDE(ctx context.Context, removeIp string) error {
	if removeIp == "" {
		klog.Infof("VIP is empty, hence not removing anything from RDE")
		return nil
	}
	if rm.ClusterID == "" || strings.HasPrefix(rm.ClusterID, NoRdePrefix) {
		klog.Infof("ClusterID [%s] is empty or generated, hence not removing VIP [%s] from RDE",
			rm.ClusterID, removeIp)
		return nil
	}

	numRetries := 10
	for i := 0; i < numRetries; i++ {
		currIps, etag, defEnt, err := rm.GetRDEVirtualIps(ctx)
		if err != nil {
			return fmt.Errorf("error getting current vips: [%v]", err)
		}
		// currIps is guaranteed not to be nil by GetRDEVirtualIps
		if len(currIps) == 0 {
			// valid case since this could be a retry operation
			return nil
		}

		// form updated virtual ip list
		foundIdx := -1
		for idx, ip := range currIps {
			if ip == removeIp {
				foundIdx = idx
				break // for inner loop
			}
		}
		if foundIdx == -1 {
			return nil // no need to update RDE
		}
		updatedIps := append(currIps[:foundIdx], currIps[foundIdx+1:]...)

		httpResponse, err := rm.updateRDEVirtualIps(ctx, updatedIps, etag, defEnt)
		if err != nil {
			if httpResponse.StatusCode == http.StatusPreconditionFailed {
				klog.Infof("Wrong ETag while removing virtual IP [%s]", removeIp)
				continue
			}
			return fmt.Errorf("error when removing virtual ip [%s] from RDE: [%v]",
				removeIp, err)
		}
		return nil
	}

	return fmt.Errorf("unable to update rde due to incorrect etag after [%d] tries", numRetries)
}
