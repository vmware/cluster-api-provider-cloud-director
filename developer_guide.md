# CAPVCD Developer guide


Install
* 

## Upgrades

### Steps to Create New APIs with Version `v1beta2`

1. Create new API files with the desired version (e.g., `v1beta2`) in the `api` directory of your project. 
   1.1. Run `kubebuilder init --domain cluster.x-k8s.io` command (optional) 
   1.2. Run `kubebuilder create api --group infrastructure --version v1beta1 --kind VCDMachine` command 
   1.3. Run `kubebuilder create api --group infrastructure --version v1beta1 --kind VCDCluster` command 
   1.4. Run `kubebuilder create api --group infrastructure --version v1beta1 --kind VCDMachineTemplate` command 
2. Copy these files to `./api/v1beta2/*`.
3. Add the `+kubebuilder:storageversion` comment to each file in the `v1beta2` folder (e.g., `VCDMachine`, `VCDCluster`, `VCDMachineTemplate`).

This will create a new API version `v1beta2` with your desired objects defined in the API files and mark it as the storage version for the CRD for conversion.

### Steps to enable clusterctl upgrade command






Dev setup
*

RDE management
* 

Core CAPI and Clusterctl matrix
* 

Pre-release check-list
*

Post-release check-list
*



