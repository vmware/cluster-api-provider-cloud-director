/*
   Copyright 2021 VMware, Inc.
   SPDX-License-Identifier: Apache-2.0
*/

package vcdsdk

import (
	"bytes"
	"context"
	"fmt"
	"github.com/apparentlymart/go-cidr/cidr"
	"k8s.io/klog"
	"net"
)

type IPRange struct {
	StartIP string
	EndIP string
}

// GetUnusedExternalIPAddress returns the first unused IP address in the gateway from an ipamSubnet
// There are races here since there is no 'acquisition' of an IP. However, since k8s retries, it will
// be correct.
func (gm *GatewayManager) GetUnusedExternalIPAddress(ctx context.Context, allowedIPAMSubnetStr string) (string, error) {
	client := gm.Client
	if gm.GatewayRef == nil {
		return "", fmt.Errorf("gateway reference should not be nil")
	}

	// Overall procedure
	// 1. Get all IP ranges in gateway
	// 2. Get all used IP addresses in gateway
	// Depending on whether allowedIPAMSubnetStr is empty or not we have two options:
	// If allowedIPAMSubnetStr is NOT empty:
	//		3. Loop through allowed ip addresses in allowedIPAMSubnetStr
	//		4. Check if the IP address is unused
	//		5. Verify that the IP address is in at least one of the gateway ranges
	// If allowedIPAMSubnetStr is empty:
	// 		3. Loop through all IPs in the allowed ip ranges in the gateway
	// 		4. Check if the IP address is unused
	// Note: This is not the best approach and can be optimized further by skipping ranges.

	// 1. Get all IP ranges in gateway
	edgeGW, resp, err := client.APIClient.EdgeGatewayApi.GetEdgeGateway(ctx, gm.GatewayRef.Id)
	if err != nil {
		return "", fmt.Errorf("unable to retrieve edge gateway details for [%s]: resp [%+v]: [%v]",
			gm.GatewayRef.Name, resp, err)
	}

	ipRangeList := make([]IPRange, 0)
	for _, edgeGWUplink := range edgeGW.EdgeGatewayUplinks {
		for _, subnet := range edgeGWUplink.Subnets.Values {
			if subnet.IpRanges == nil {
				continue
			}
			for _, ipRangeValue := range subnet.IpRanges.Values {
				ipRangeList = append(ipRangeList, IPRange{
					StartIP: ipRangeValue.StartAddress,
					EndIP: ipRangeValue.EndAddress,
				})
			}
		}
	}
	if len(ipRangeList) == 0 {
		return "", fmt.Errorf("unable to get any ipRanges in gateway")
	}

	// 2. Get all used IP addresses in gateway
	usedIPAddresses := make(map[string]bool)
	pageNum := int32(1)
	for {
		gwUsedIPAddresses, resp, err := client.APIClient.EdgeGatewayApi.GetUsedIpAddresses(ctx, pageNum, 25,
			gm.GatewayRef.Id, nil)
		if err != nil {
			return "", fmt.Errorf("unable to get used IP addresses of gateway [%s]: [%+v]: [%v]",
				gm.GatewayRef.Name, resp, err)
		}
		if len(gwUsedIPAddresses.Values) == 0 {
			break
		}

		for _, gwUsedIPAddress := range gwUsedIPAddresses.Values {
			usedIPAddresses[gwUsedIPAddress.IpAddress] = true
		}

		pageNum++
	}

	freeIP := ""
	if allowedIPAMSubnetStr != "" {
		_, allowedIPAMSubnet, err := net.ParseCIDR(allowedIPAMSubnetStr)
		if err != nil {
			return "", fmt.Errorf("unable to parse CIDR [%s] into a subnet: [%v]", allowedIPAMSubnetStr, err)
		}
		allowedStartIP, allowedEndIP := cidr.AddressRange(allowedIPAMSubnet)
		// 3. Loop through allowed ip addresses in the user-specified IPAM Subnet
		// 4. Check if the IP address is unused &&
		// 5. Verify that the IP address is in at least one of the gateway ranges
		// 3-5 performed in the following function
		freeIP, err = getUnusedIPAddressInAllowedRange(allowedStartIP.String(), allowedEndIP.String(),
			usedIPAddresses, &ipRangeList)
		if err != nil {
			return "", fmt.Errorf("unable to find unused IP from [%s-%s] in IP ranges [%v]",
				allowedStartIP.String(), allowedEndIP.String(), ipRangeList)
		}
	} else {
		// 3. Loop through all IPs in the allowed ip ranges in the gateway
		// 4. Check if the IP address is unused
		// 3-4 performed in the following function
		freeIP, err = getUnusedIPAddressInRange(usedIPAddresses, ipRangeList)
		if err != nil {
			return "", fmt.Errorf("unable to find unused IP in IP ranges [%v]: [%v]",
				ipRangeList, err)
		}
	}

	if freeIP == "" {
		return "", fmt.Errorf("unable to obtain free IP from gateway [%s]; all are used",
			gm.GatewayRef.Name)
	}
	klog.Infof("Using unused IP [%s] on gateway [%v]\n", freeIP, gm.GatewayRef.Name)

	return freeIP, nil
}

func (gm *GatewayManager) GetUnusedInternalIPAddress(ctx context.Context, oneArm *OneArm) (string, error) {
	if oneArm == nil {
		return "", fmt.Errorf("unable to get unused internal IP address as oneArm is nil")
	}

	if gm.GatewayRef == nil {
		return "", fmt.Errorf("gateway reference should not be nil")
	}
	client := gm.Client

	usedIPAddresses := make(map[string]bool)
	pageNum := int32(1)
	for {
		lbVSSummaries, resp, err := client.APIClient.EdgeGatewayLoadBalancerVirtualServicesApi.GetVirtualServiceSummariesForGateway(
			ctx, pageNum, 25, gm.GatewayRef.Id, nil)
		if err != nil {
			return "", fmt.Errorf("unable to get virtual service summaries for gateway [%s]: resp: [%v]: [%v]",
				gm.GatewayRef.Name, resp, err)
		}
		if len(lbVSSummaries.Values) == 0 {
			break
		}
		for _, lbVSSummary := range lbVSSummaries.Values {
			usedIPAddresses[lbVSSummary.VirtualIpAddress] = true
		}

		pageNum++
	}

	freeIP, err := getUnusedIPAddressInAllowedRange(oneArm.StartIP, oneArm.EndIP, usedIPAddresses, nil)
	if err != nil {
		return "", fmt.Errorf("error in finding unused IP address in range [%s-%s]: [%v]",
			oneArm.StartIP, oneArm.EndIP, err)
	}
	if freeIP == "" {
		return "", fmt.Errorf("unable to find unused IP address in range [%s-%s]",
			oneArm.StartIP, oneArm.EndIP)
	}

	return freeIP, nil
}

func getUnusedIPAddressInRange(usedIPAddresses map[string]bool, ipRangeList []IPRange) (string, error) {
	for _, ipRange := range ipRangeList {
		startIP, _, err := net.ParseCIDR(fmt.Sprintf("%s/32", ipRange.StartIP))
		if err != nil {
			return "", fmt.Errorf("unable to parse start IP CIDR of range [%v]: [%v]", ipRange, err)
		}

		endIP, _, err := net.ParseCIDR(fmt.Sprintf("%s/32", ipRange.EndIP))
		if err != nil {
			return "", fmt.Errorf("unable to parse start IP CIDR of range [%v]: [%v]", ipRange, err)
		}
		endIP = cidr.Inc(endIP)

		currIP := startIP
		for !currIP.Equal(endIP) {
			if !usedIPAddresses[currIP.String()] {
				return currIP.String(), nil
			}

			currIP = cidr.Inc(currIP)
		}
	}

	return "", nil
}

func getUnusedIPAddressInAllowedRange(startIPAddress string, endIPAddress string,
	usedIPAddresses map[string]bool, ipRangeListPtr *[]IPRange) (string, error) {

	// 3. Loop through allowed ip addresses in gateway's ipamSubnet
	// 4. Check if the IP address is unused &&
	// 5. Verify that the IP address is in at least one of the gateway ranges

	startIP, _, err := net.ParseCIDR(fmt.Sprintf("%s/32", startIPAddress))
	if err != nil {
		return "", fmt.Errorf("unable to parse start IP CIDR [%s]: [%v]", startIPAddress, err)
	}

	endIP, _, err := net.ParseCIDR(fmt.Sprintf("%s/32", endIPAddress))
	if err != nil {
		return "", fmt.Errorf("unable to parse end IP CIDR [%s]: [%v]", endIPAddress, err)
	}
	endIP = cidr.Inc(endIP)

	freeIP := ""
	currIP := startIP
	for !currIP.Equal(endIP) {
		if !usedIPAddresses[currIP.String()] && checkIfIPInRanges(currIP.String(), ipRangeListPtr) {
			freeIP = currIP.String()
			break
		}

		currIP = cidr.Inc(currIP)
	}

	return freeIP, nil
}

// checkIfIPInRanges checks if the ipStr is in the list of ranges provided in ipRangeListPtr
// If ipRangeListPtr is nil, ipStr is found. This was the simplest way to not have a check.
func checkIfIPInRanges(ipStr string, ipRangeListPtr *[]IPRange) bool {
	if ipRangeListPtr == nil {
		return true
	}
	ipRangeList := *ipRangeListPtr
	ip := net.ParseIP(ipStr)
	for _, ipRange := range ipRangeList {
		startIP := net.ParseIP(ipRange.StartIP)
		endIP := net.ParseIP(ipRange.EndIP)

		if bytes.Compare(ip, startIP) >= 0 && bytes.Compare(ip, endIP) <= 0 {
			return true
		}
	}

	return false
}
