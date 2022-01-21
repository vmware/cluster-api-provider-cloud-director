# VCD Setup

## Provider Setup

### Avi controller, NSX-T Cloud Setup
The LoadBalancers fronting the Multimaster workload clusters will need a preconfigured Avi Controller, NSX-T Cloud and Avi Service Engine Group. This is a provider operation.
The Service Engine Group (SEG) should be created as Dedicated and one SEG should be allocated per Edge Gateway in order to ensure that Load Balancers used by Tenants are well-isolated from each other.
The LoadBalancer section of the Edge Gateway for a Tenant should be enabled, and the appropriate Service Engine Group(s) should be configured into the Edge Gateway. This will be used to create Virtual Services when a LoadBalancer request is made from Kubernetes.

### CAPVCD RDE registration and right bundles publishment
CAPVCD RDE registration a) via CSE b) REST API
* Publish CAPVCD right bundle to the tenant organizations
* Using UI, Create a right bundle and publish to the org (or) add the below rights to the Default Right Bundle
* User > Manage user's own API token
* Organization VDC > Create a Shared Disk
* vApp > Preserve all ExtraConfig Elements during OVA Import and Export
* General > Manage Certificates Library

## Tenant Setup
A ServiceEngineGroup needs to be added to the gateway of the OVDC within which the Kubernetes cluster is to be created. The overall steps to achieve that are documented at Enable Load Balancer on an NSX-T Data Center Edge Gateway
Create and publish the desired sizing policies on the chosen ovdc(s)
Upload TKGm templates via a) CSE b) VCD UI

* SNAT rule on the gateway - This just means internal IP range need to have outbound access?
* DNS set up on the OVDC network - is this needed in the CU environments?
* Create a CAPVCD user role with the above mentioned rights in addition to the vApp Author privileges

