package capisdk

import (
	"context"
	"fmt"
	"github.com/vmware/cloud-provider-for-cloud-director/pkg/vcdsdk"
	"github.com/vmware/cluster-api-provider-cloud-director/release"
	"k8s.io/klog"
)

type CapvcdGatewayManager struct {
	gatewayManager *vcdsdk.GatewayManager
}

func GetVirtualServiceNamePrefix(clusterName string, clusterID string) string {
	return clusterName + "-" + clusterID
}

func GetLoadBalancerPoolNamePrefix(clusterName string, clusterID string) string {
	return clusterName + "-" + clusterID
}

func GetVirtualServiceNameUsingPrefix(virtualServiceNamePrefix string, portSuffix string) string {
	return fmt.Sprintf("%s-%s", virtualServiceNamePrefix, portSuffix)
}

func GetLoadBalancerPoolNameUsingPrefix(lbPoolNamePrefix string, portSuffix string) string {
	return fmt.Sprintf("%s-%s", lbPoolNamePrefix, portSuffix)
}

func NewCapvcdGatewayManager(ctx context.Context, client *vcdsdk.Client, networkName string, ipamSubnet string) (*CapvcdGatewayManager, error) {
	gatewayManager, err := vcdsdk.NewGatewayManager(ctx, client, networkName, ipamSubnet)
	if err != nil {
		return nil, fmt.Errorf("error creating gateway manager: [%v]", err)
	}
	return &CapvcdGatewayManager{
		gatewayManager: gatewayManager,
	}, nil
}

// TODO: remove CreateL4LoadBalancer, UpdateLoadBalancer and DeleteLoadBalancer and use the functions defined in CPI

func (capvcdGatewayManager *CapvcdGatewayManager) CreateL4LoadBalancer(ctx context.Context, virtualServiceNamePrefix string,
	lbPoolNamePrefix string, ips []string, tcpPort int32, externalTcpPort int32, oneArm *vcdsdk.OneArm, clusterID string) (string, error) {

	if capvcdGatewayManager.gatewayManager == nil {
		return "", fmt.Errorf("referece to gateway manager is nil")
	}
	capvcdGatewayManager.gatewayManager.Client.RWLock.Lock()
	defer capvcdGatewayManager.gatewayManager.Client.RWLock.Unlock()

	rdeManager := vcdsdk.NewRDEManager(capvcdGatewayManager.gatewayManager.Client, clusterID, StatusComponentNameCAPVCD, release.CAPVCDVersion)
	if tcpPort == 0 {
		// nothing to do here
		klog.V(3).Infof("There is no tcp port specified. Cannot create L4 load balancer")
		return "", fmt.Errorf("there is no tcp port specified. Cannot create L4 load balancer")
	}

	if capvcdGatewayManager.gatewayManager.GatewayRef == nil {
		return "", fmt.Errorf("capvcdGatewayManager reference should not be nil")
	}

	type PortDetails struct {
		portSuffix       string
		serviceType      string
		externalPort     int32
		internalPort     int32
		useSSL           bool
		certificateAlias string
	}
	portDetails := []PortDetails{
		{
			portSuffix:       "tcp",
			serviceType:      "TCP",
			externalPort:     externalTcpPort,
			internalPort:     tcpPort,
			useSSL:           false,
			certificateAlias: "",
		},
	}

	externalIP, err := capvcdGatewayManager.gatewayManager.GetUnusedExternalIPAddress(ctx, capvcdGatewayManager.gatewayManager.IPAMSubnet)
	if err != nil {
		return "", fmt.Errorf("unable to get unused IP address from subnet [%s]: [%v]",
			capvcdGatewayManager.gatewayManager.IPAMSubnet, err)
	}
	klog.V(3).Infof("Using external IP [%s] for virtual service\n", externalIP)

	for _, portDetail := range portDetails {
		if portDetail.internalPort == 0 {
			klog.V(3).Infof("No internal port specified for [%s], hence loadbalancer not created\n", portDetail.portSuffix)
			continue
		}

		virtualServiceName := GetVirtualServiceNameUsingPrefix(virtualServiceNamePrefix, portDetail.portSuffix)
		lbPoolName := GetLoadBalancerPoolNameUsingPrefix(lbPoolNamePrefix, portDetail.portSuffix)

		vsSummary, err := capvcdGatewayManager.gatewayManager.GetVirtualService(ctx, virtualServiceName)
		if err != nil {
			return "", fmt.Errorf("unexpected error while querying for virtual service [%s]: [%v]",
				virtualServiceName, err)
		}
		if vsSummary != nil {
			if vsSummary.LoadBalancerPoolRef.Name != lbPoolName {
				return "", fmt.Errorf("virtual service [%s] found with unexpected loadbalancer pool [%s]",
					virtualServiceName, lbPoolName)
			}

			klog.Infof("LoadBalancer Virtual Service [%s] already exists", virtualServiceName)
			if err = capvcdGatewayManager.gatewayManager.CheckIfVirtualServiceIsPending(ctx, virtualServiceName); err != nil {
				return "", err
			}

			dnatRuleName := vcdsdk.GetDNATRuleName(virtualServiceName)
			dnatRuleRef, err := capvcdGatewayManager.gatewayManager.GetNATRuleRef(ctx, dnatRuleName)
			if err != nil {
				return "", fmt.Errorf("failed to fetch DNAT rule [%s]: [%v]", dnatRuleName, err)
			}
			// Add virtual service to VCDResourceSet
			err = rdeManager.AddToVCDResourceSet(ctx, vcdsdk.ComponentCAPVCD, vcdsdk.VcdResourceVirtualService,
				virtualServiceName, vsSummary.Id, map[string]interface{}{
					"virtualIP": dnatRuleRef.ExternalIP,
				})
			if err != nil {
				return "", fmt.Errorf("failed to add VCD resource [%s] of type [%s] to VCDResourceSet of RDE [%s]: [%v]",
					vsSummary.Name, vcdsdk.VcdResourceVirtualService, clusterID, err)
			}
			continue
		}

		virtualServiceIP := externalIP
		if oneArm != nil {
			internalIP, err := capvcdGatewayManager.gatewayManager.GetUnusedInternalIPAddress(ctx, oneArm)
			if err != nil {
				return "", fmt.Errorf("unable to get internal IP address for one-arm mode: [%v]", err)
			}

			dnatRuleName := vcdsdk.GetDNATRuleName(virtualServiceName)

			// create app port profile
			appPortProfileName := vcdsdk.GetAppPortProfileName(dnatRuleName)
			appPortProfile, err := capvcdGatewayManager.gatewayManager.CreateAppPortProfile(appPortProfileName, portDetail.externalPort)
			if err != nil {
				return "", fmt.Errorf("failed to create App Port Profile: [%v]", err)
			}
			if appPortProfile == nil || appPortProfile.NsxtAppPortProfile == nil {
				return "", fmt.Errorf("creation of app port profile succeeded but app port profile is empty")
			}
			// Add App Port Profile to VCDResourceSet
			err = rdeManager.AddToVCDResourceSet(ctx, vcdsdk.ComponentCAPVCD, vcdsdk.VcdResourceAppPortProfile, appPortProfileName,
				appPortProfile.NsxtAppPortProfile.ID, nil)
			if err != nil {
				return "", fmt.Errorf("failed to Add VCD Resource [%s] of type [%s] to VCDResourceSet of RDE [%s]: [%v]",
					appPortProfileName, vcdsdk.VcdResourceAppPortProfile, clusterID, err)
			}

			err = capvcdGatewayManager.gatewayManager.CreateDNATRule(ctx, dnatRuleName, externalIP,
				internalIP, portDetail.externalPort, portDetail.internalPort, appPortProfile)
			if err != nil {
				return "", fmt.Errorf("unable to create dnat rule [%s] => [%s]: [%v]",
					externalIP, internalIP, err)
			}
			// use the internal IP to create virtual service
			virtualServiceIP = internalIP

			dnatRuleRef, err := capvcdGatewayManager.gatewayManager.GetNATRuleRef(ctx, dnatRuleName)
			if err != nil {
				return "", fmt.Errorf("unable to retrieve created dnat rule [%s]: [%v]", dnatRuleName, err)
			}
			if dnatRuleRef == nil {
				return "", fmt.Errorf("retrieved dnat rule ref is nil")
			}

			// Add DNAT Rule to VCDResourceSet
			err = rdeManager.AddToVCDResourceSet(ctx, vcdsdk.ComponentCAPVCD, vcdsdk.VcdResourceDNATRule,
				dnatRuleName, dnatRuleRef.ID, nil)
			if err != nil {
				return "", fmt.Errorf("failed to add VCD Resource [%s] of type [%s] to VCDResourceSet of RDE [%s]: [%v]",
					dnatRuleName, vcdsdk.VcdResourceDNATRule, clusterID, err)
			}
			externalIP = dnatRuleRef.ExternalIP
		}

		segRef, err := capvcdGatewayManager.gatewayManager.GetLoadBalancerSEG(ctx)
		if err != nil {
			return "", fmt.Errorf("unable to get service engine group from edge [%s]: [%v]",
				capvcdGatewayManager.gatewayManager.GatewayRef.Name, err)
		}

		lbPoolRef, err := capvcdGatewayManager.gatewayManager.CreateLoadBalancerPool(ctx, lbPoolName, ips, portDetail.internalPort)
		if err != nil {
			return "", fmt.Errorf("unable to create load balancer pool [%s]: [%v]", lbPoolName, err)
		}

		// Add LoadBalancerPool to VCDResourceSet
		err = rdeManager.AddToVCDResourceSet(ctx, vcdsdk.ComponentCAPVCD, vcdsdk.VcdResourceLoadBalancerPool,
			lbPoolRef.Name, lbPoolRef.Id, nil)
		if err != nil {
			return "", fmt.Errorf("failed to add VCD Resource [%s] of type [%s] to VCDResourceSet of RDE [%s]: [%v]",
				lbPoolRef.Name, vcdsdk.VcdResourceLoadBalancerPool, rdeManager.ClusterID, err)
		}
		virtualServiceRef, err := capvcdGatewayManager.gatewayManager.CreateVirtualService(ctx, virtualServiceName, lbPoolRef, segRef,
			virtualServiceIP, portDetail.serviceType, portDetail.externalPort, portDetail.useSSL, portDetail.certificateAlias)
		if err != nil {
			return "", fmt.Errorf("unable to create virtual service [%s] with address [%s:%d]: [%v]",
				virtualServiceName, virtualServiceIP, portDetail.externalPort, err)
		}
		// Add virtual service to vcd resource set
		err = rdeManager.AddToVCDResourceSet(ctx, vcdsdk.ComponentCAPVCD, vcdsdk.VcdResourceVirtualService,
			virtualServiceRef.Name, virtualServiceRef.Id, map[string]interface{}{
				"virtualIP": externalIP,
			})
		if err != nil {
			return "", fmt.Errorf("failed to add virtual service [%s] to VCDResourceSet of RDE [%s]", virtualServiceRef.Name, rdeManager.ClusterID)
		}
		klog.V(3).Infof("Created Load Balancer with virtual service [%v], pool [%v] on capvcdGatewayManager [%s]\n",
			virtualServiceRef, lbPoolRef, capvcdGatewayManager.gatewayManager.GatewayRef.Name)
	}

	return externalIP, nil
}

// DeleteLoadBalancer : create a new load balancer pool and virtual service pointing to it
func (capvcdGatewayManager *CapvcdGatewayManager) DeleteLoadBalancer(ctx context.Context, virtualServiceNamePrefix string,
	lbPoolNamePrefix string, oneArm *vcdsdk.OneArm, clusterID string) error {

	if capvcdGatewayManager.gatewayManager == nil {
		return fmt.Errorf("reference to gateway manager is nil")
	}

	client := capvcdGatewayManager.gatewayManager.Client
	client.RWLock.Lock()
	defer client.RWLock.Unlock()

	rdeManager := vcdsdk.NewRDEManager(capvcdGatewayManager.gatewayManager.Client, clusterID, StatusComponentNameCAPVCD, release.CAPVCDVersion)

	// TODO: try to continue in case of errors
	var err error

	for _, suffix := range []string{"http", "https", "tcp"} {

		virtualServiceName := GetVirtualServiceNameUsingPrefix(virtualServiceNamePrefix, suffix)
		lbPoolName := GetLoadBalancerPoolNameUsingPrefix(lbPoolNamePrefix, suffix)

		err = capvcdGatewayManager.gatewayManager.DeleteVirtualService(ctx, virtualServiceName, false)
		if err != nil {
			return fmt.Errorf("unable to delete virtual service [%s]: [%v]", virtualServiceName, err)
		}
		err = rdeManager.RemoveFromVCDResourceSet(ctx, vcdsdk.ComponentCAPVCD, vcdsdk.VcdResourceVirtualService, virtualServiceName)
		if err != nil {
			return fmt.Errorf("failed to remove VCD Resource [%s] of type [%s] from VCDResourceSet of RDE [%s]: [%v]",
				virtualServiceName, vcdsdk.VcdResourceVirtualService, clusterID, err)
		}

		err = capvcdGatewayManager.gatewayManager.DeleteLoadBalancerPool(ctx, lbPoolName, false)
		if err != nil {
			return fmt.Errorf("unable to delete load balancer pool [%s]: [%v]", lbPoolName, err)
		}
		err = rdeManager.RemoveFromVCDResourceSet(ctx, vcdsdk.ComponentCAPVCD, vcdsdk.VcdResourceLoadBalancerPool, lbPoolName)
		if err != nil {
			return fmt.Errorf("failed to remove VCD Resource [%s] of type [%s] from VCDResourceSet of RDE [%s]: [%v]",
				lbPoolName, vcdsdk.VcdResourceLoadBalancerPool, clusterID, err)
		}

		if oneArm != nil {
			// delete dnat rule
			dnatRuleName := vcdsdk.GetDNATRuleName(virtualServiceName)
			err = capvcdGatewayManager.gatewayManager.DeleteDNATRule(ctx, dnatRuleName, false)
			if err != nil {
				return fmt.Errorf("unable to delete dnat rule [%s]: [%v]", dnatRuleName, err)
			}
			err = rdeManager.RemoveFromVCDResourceSet(ctx, vcdsdk.ComponentCAPVCD, vcdsdk.VcdResourceDNATRule, dnatRuleName)
			if err != nil {
				return fmt.Errorf("failed to remove VCD Resource [%s] of type [%s] from VCDResourceSet of RDE [%s]: [%v]",
					dnatRuleName, vcdsdk.VcdResourceDNATRule, clusterID, err)
			}

			// delete app port profile
			appPortProfileName := vcdsdk.GetAppPortProfileName(dnatRuleName)
			err = capvcdGatewayManager.gatewayManager.DeleteAppPortProfile(appPortProfileName, false)
			if err != nil {
				return fmt.Errorf("failed to remove VCD Resource [%s] of type [%s] from VCDResourceSet of RDE [%s]: [%v]",
					appPortProfileName, vcdsdk.VcdResourceAppPortProfile, clusterID, err)
			}
		}
	}

	return nil
}

func (capvcdGatewayManager *CapvcdGatewayManager) UpdateLoadBalancer(ctx context.Context, lbPoolName string,
	ips []string, internalPort int32) error {

	if capvcdGatewayManager.gatewayManager == nil {
		return fmt.Errorf("referece to gateway manager is nil")
	}

	client := capvcdGatewayManager.gatewayManager.Client
	client.RWLock.Lock()
	defer client.RWLock.Unlock()

	_, err := capvcdGatewayManager.gatewayManager.UpdateLoadBalancerPool(ctx, lbPoolName, ips, internalPort)
	if err != nil {
		return fmt.Errorf("unable to update load balancer pool [%s]: [%v]", lbPoolName, err)
	}

	return nil
}
