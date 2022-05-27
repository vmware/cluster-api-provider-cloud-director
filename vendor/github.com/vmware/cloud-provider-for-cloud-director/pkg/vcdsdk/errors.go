package vcdsdk

import (
	"fmt"
	"runtime/debug"
)

type VirtualServicePendingError struct {
	VirtualServiceName string
}

type VirtualServiceBusyError struct {
	VirtualServiceName string
}

type LoadBalancerPoolBusyError struct {
	LBPoolName string
}

type GatewayBusyError struct {
	GatewayName string
}

func (vsError *VirtualServicePendingError) Error() string {
	return fmt.Sprintf("virtual service [%s] is in Pending state", vsError.VirtualServiceName)
}

func NewVirtualServicePendingError(virtualServiceName string) *VirtualServicePendingError {
	return &VirtualServicePendingError{
		VirtualServiceName: virtualServiceName,
	}
}

func (vsError *VirtualServiceBusyError) Error() string {
	return fmt.Sprintf("virtual service [%s] is busy", vsError.VirtualServiceName)
}

func NewVirtualServiceBusyError(virtualServiceName string) *VirtualServiceBusyError {
	return &VirtualServiceBusyError{
		VirtualServiceName: virtualServiceName,
	}
}

func (lbPoolError *LoadBalancerPoolBusyError) Error() string {
	return fmt.Sprintf("load balancer pool [%s] is busy", lbPoolError.LBPoolName)
}

func NewLBPoolBusyError(lbPoolName string) *LoadBalancerPoolBusyError {
	return &LoadBalancerPoolBusyError{
		LBPoolName: lbPoolName,
	}
}

func (gatewayBusyError *GatewayBusyError) Error() string {
	return fmt.Sprintf("gateway [%s] is busy", gatewayBusyError.GatewayName)
}

func NewGatewayBusyError(gatewayName string) *GatewayBusyError {
	return &GatewayBusyError{
		GatewayName: gatewayName,
	}
}

// NoRDEError is an error used when the InfraID value in the VCDCluster object does not point to a valid RDE in VCD
type NoRDEError struct {
	msg string
}

func (nre *NoRDEError) Error() string {
	if nre == nil {
		return fmt.Sprintf("error is unexpectedly nil at stack [%s]", string(debug.Stack()))
	}
	return nre.msg
}

func NewNoRDEError(message string) *NoRDEError {
	return &NoRDEError{msg: message}
}
