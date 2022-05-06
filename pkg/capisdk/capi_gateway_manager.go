package capisdk

import (
	"context"
	"fmt"
	"github.com/vmware/cloud-provider-for-cloud-director/pkg/vcdsdk"
	"k8s.io/klog"
)

type CapvcdGatewayManager struct {
	gatewayManager *vcdsdk.GatewayManager
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

func (capvcdGatewayManager *CapvcdGatewayManager) CreateL4LoadBalancer(ctx context.Context, virtualServiceNamePrefix string,
	lbPoolNamePrefix string, ips []string, tcpPort int32, externalTcpPort int32, oneArm *vcdsdk.OneArm) (string, error) {

	if capvcdGatewayManager.gatewayManager == nil {
		return "", fmt.Errorf("referece to gateway manager is nil")
	}
	capvcdGatewayManager.gatewayManager.Client.RWLock.Lock()
	defer capvcdGatewayManager.gatewayManager.Client.RWLock.Unlock()

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

		virtualServiceName := fmt.Sprintf("%s-%s", virtualServiceNamePrefix, portDetail.portSuffix)
		lbPoolName := fmt.Sprintf("%s-%s", lbPoolNamePrefix, portDetail.portSuffix)

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
			continue
		}

		virtualServiceIP := externalIP
		if oneArm != nil {
			internalIP, err := capvcdGatewayManager.gatewayManager.GetUnusedInternalIPAddress(ctx, oneArm)
			if err != nil {
				return "", fmt.Errorf("unable to get internal IP address for one-arm mode: [%v]", err)
			}

			dnatRuleName := fmt.Sprintf("dnat-%s", virtualServiceName)
			err = capvcdGatewayManager.gatewayManager.CreateDNATRule(ctx, dnatRuleName, externalIP,
				internalIP, portDetail.externalPort, portDetail.internalPort)
			if err != nil {
				return "", fmt.Errorf("unable to create dnat rule [%s] => [%s]: [%v]",
					externalIP, internalIP, err)
			}
			// use the internal IP to create virtual service
			virtualServiceIP = internalIP
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

		virtualServiceRef, err := capvcdGatewayManager.gatewayManager.CreateVirtualService(ctx, virtualServiceName, lbPoolRef, segRef,
			virtualServiceIP, portDetail.serviceType, portDetail.externalPort, portDetail.useSSL, portDetail.certificateAlias)
		if err != nil {
			return "", fmt.Errorf("unable to create virtual service [%s] with address [%s:%d]: [%v]",
				virtualServiceName, virtualServiceIP, portDetail.externalPort, err)
		}
		klog.V(3).Infof("Created Load Balancer with virtual service [%v], pool [%v] on capvcdGatewayManager [%s]\n",
			virtualServiceRef, lbPoolRef, capvcdGatewayManager.gatewayManager.GatewayRef.Name)
	}

	return externalIP, nil
}

// DeleteLoadBalancer : create a new load balancer pool and virtual service pointing to it
func (capvcdGatewayManager *CapvcdGatewayManager) DeleteLoadBalancer(ctx context.Context, virtualServiceNamePrefix string,
	lbPoolNamePrefix string, oneArm *vcdsdk.OneArm) error {

	if capvcdGatewayManager.gatewayManager == nil {
		return fmt.Errorf("referece to gateway manager is nil")
	}

	client := capvcdGatewayManager.gatewayManager.Client
	client.RWLock.Lock()
	defer client.RWLock.Unlock()

	// TODO: try to continue in case of errors
	var err error

	for _, suffix := range []string{"http", "https", "tcp"} {

		virtualServiceName := fmt.Sprintf("%s-%s", virtualServiceNamePrefix, suffix)
		lbPoolName := fmt.Sprintf("%s-%s", lbPoolNamePrefix, suffix)

		err = capvcdGatewayManager.gatewayManager.DeleteVirtualService(ctx, virtualServiceName, false)
		if err != nil {
			return fmt.Errorf("unable to delete virtual service [%s]: [%v]", virtualServiceName, err)
		}

		err = capvcdGatewayManager.gatewayManager.DeleteLoadBalancerPool(ctx, lbPoolName, false)
		if err != nil {
			return fmt.Errorf("unable to delete load balancer pool [%s]: [%v]", lbPoolName, err)
		}

		if oneArm != nil {
			dnatRuleName := fmt.Sprintf("dnat-%s", virtualServiceName)
			err = capvcdGatewayManager.gatewayManager.DeleteDNATRule(ctx, dnatRuleName, false)
			if err != nil {
				return fmt.Errorf("unable to delete dnat rule [%s]: [%v]", dnatRuleName, err)
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
