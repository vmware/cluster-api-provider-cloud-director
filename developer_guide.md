# CAPVCD Developer guide

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
1. Update [metadata.yaml](https://github.com/vmware/cluster-api-provider-cloud-director/blob/main/metadata.yaml) with the core capi contract of the new CAPVCD
2. Upgrade clusterctl version to the 
CAPVCD upgrade can now be executed using the command -

`clusterctl upgrade apply --contract v1beta1`

### Steps to enable CAPVCD handle other supported versions

1. To enable conversion between different versions of the custom resource definition (CRD), we need to define a way to convert between the versions. This is done by following the Hub and Spokes model, where the stored version (e.g., v1beta2) is marked as the Hub and all other versions (e.g., v1alpha4, v1beta1) are marked as Spokes.
2. To mark the `v1beta1` types as Hub, go to the api/v1beta1 directory and create a vcdcluster_conversion.go file next to vcdcluster_types.go for each type. In each conversion file, add the empty method func (*VCDCluster) Hub() {} to serve as the Hub marker.
3. To mark the `v1alpha4` and any other older versions as Spokes, go to the api/v1alpha4 directory and create a vcdcluster_conversion.go file next to vcdcluster_types.go for each type. Implement the convertible interface and provide the conversion methods:

```
// ConvertTo converts this (v1alpha4)VCDCluster to the Hub version (v1beta1).
func (src *VCDCluster) ConvertTo(dstRaw conversion.Hub) error {
dst := dstRaw.(*v1beta1.VCDCluster)

    dst.Spec.ControlPlaneEnpoint := src.Spec.ControlPlaneEndpoint
    
    //rote conversion
    return nil
}

// ConvertFrom converts from the Hub version (v1beta1) to this version (v1alpha4).
func (dst *VCDCluster) ConvertFrom(srcRaw conversion.Hub) error {
src := srcRaw.(*v1beta1.VCDCluster)
dst.Spec.ControlPlaneEnpoint := src.Spec.ControlPlaneEndpoint
//rote conversion
return nil
}
```

These methods handle the conversion of the VCDCluster type from the v1alpha4 Spoke version to the v1beta1 Hub version and vice versa. The conversion logic needs to be implemented for all the fields of the type to convert between the versions.

### Steps to enable conversion webhooks in CAPVCD
The kubebuilder create webhook command generates the scaffolding for a webhook in your Kubernetes operator project, including the conversion logic for the specified CRD.
When running the following command:
`kubebuilder create webhook --group infrastructure --version v1beta2 --kind VCDCluster --conversion`
CAPVCD will create a file named `VCDCluster_webhook.go` in the `/api/v1beta2/` directory. This file will contain the conversion logic for the VCDCluster resource, which is defined in the infrastructure/v1beta1 API group and version.

### Steps to enable validation and mutation webhooks in CAPVCD 

(This step may be needed if any new fields being introduced require default values or validation)

1. The following commands will create validation and mutation webhooks for the `VCDCluster` and `VCDMachine` resources, respectively, in the `infrastructure/v1beta2` API group and version:

```
kubebuilder create webhook --group infrastructure --version v1beta2 --kind VCDCluster --defaulting --programmatic-validation
kubebuilder create webhook --group infrastructure --version v1beta2 --kind VCDMachine --defaulting --programmatic-validation
```

2. If the webhooks already exist but need to be re-created, you can use the `--force` flag with the above commands.

3. Run `make manifests` to update the [`config/webhook/manifests.yaml`](https://github.com/vmware/cluster-api-provider-cloud-director/blob/main/config/webhook/manifests.yaml) file, which contains the Kubernetes resources necessary to deploy the webhooks.

4. Run `make release-manifests` to update the [`templates/infrastructure-components.yaml`](https://github.com/vmware/cluster-api-provider-cloud-director/blob/main/templates/infrastructure-components.yaml) file, which defines the resources for deploying your operator, including the webhook configuration.

5. Ensure that the `PROJECT` file at the root of your repository accurately reflects the versions of the API objects and their associated webhooks, as this file is used by the `kubebuilder` CLI to determine the API versions present and their associated webhooks.

### Steps to Enable Webhooks in Our CRDs
By default, webhooks are disabled in CRDs.
1. Enable patches/webhook_in_<kind>.yaml and patches/cainjection_in_<kind>.yaml in the config/crd/kustomization.yaml file.
2. Enable ../certmanager and ../webhook directories under the bases section in the config/default/kustomization.yaml file.
3. Enable manager_webhook_patch.yaml under the patches section in the config/default/kustomization.yaml file.
4. Enable all the vars under the CERTMANAGER section in the config/default/kustomization.yaml file.
5. Additionally, if present in our Makefile, we’ll need to set the CRD_OPTIONS variable to just "crd", removing the trivialVersions option.

### Writing Converters
1. Create `v1alpha4/doc.go` file and add below content:
```
// +groupName=infrastructure.cluster.x-k8s.io
// +k8s:conversion-gen=github.com/vmware/cluster-api-provider-cloud-director/api/v1beta1
package v1alpha4
```
2. Under `v1alpha4/groupversion_info.go` var, add below:
```
   localSchemeBuilder = SchemeBuilder.SchemeBuilder
```
3. Add a file boilerplate header - ./boilerplate.go.txt.
4. Run make conversion. This downloads the conversion-gen binary to the "bin/" directory.
5. Run the below command to autogenerate straightforward conversions:
```
make generate_conversions
```
6. Below is the sample output. It says some extra conversion logic is required for defaultStorageOptions and RDEId, which are newly added in v1beta1. You should see something similar for the newly added/deleted/renamed properties:
```

E0406 15:21:22.103396   22111 conversion.go:755] Warning: could not find nor generate a final Conversion function for github.com/vmware/cluster-api-provider-cloud-director/api/v1beta1.VCDClusterSpec -> ./api/v1alpha4.VCDClusterSpec
E0406 15:21:22.103633   22111 conversion.go:756]   the following fields need manual conversion:
E0406 15:21:22.103638   22111 conversion.go:758]       - DefaultStorageClassOptions
sayloo@sayloo-a02 cluster-api-provider-cloud-director % ./conversion-gen --input-dirs=./api/v1alpha4 --output-file-base=zz_generated.conversion --go-header-file=./boilerplate.go.txt --build-tag=ignore_autogenerated_conversions
E0406 15:51:05.453198   29149 conversion.go:755] Warning: could not find nor generate a final Conversion function for github.com/vmware/cluster-api-provider-cloud-director/api/v1beta1.VCDClusterSpec -> ./api/v1alpha4.VCDClusterSpec
E0406 15:51:05.453812   29149 conversion.go:756]   the following fields need manual conversion:
E0406 15:51:05.453817   29149 conversion.go:758]       - DefaultStorageClassOptions
E0406 15:51:05.453820   29149 conversion.go:758]       - RDEId
```
7. Write manual conversion logic for the above like properties. Ensure the missing implementation for autogenerated method is in the same package. DO NOT write any manual conversion logic in the zz_generated.conversion.go (it may get overwritten next time conversion-gen is run again). Use a different file like v1alpha4/conversion.go if needed.
8. Implement VCD*_conversion.go for all VCD types. For example, see valpha4/vcdcluster_conversion.go

**Notes**
1. Even if there are no warning messages in the console output, auto generated file will still contain warnings about the newly added/renamed/deleted properties. This is a way to check what properties have been edited (history)  at any given point.
2. Add the manual conversion logic only in *types_conversion.go files.
3. IDE terminal may have some issues running the conversion-gen tool. Use regular terminal.
4. Deleting the existing zz_* file and rerunning the tool will and should not cause any issues.

### Update Labels on CRDs
Update the CAPI contract under config/crd/kustomization.yaml
```
commonLabels:
  cluster.x-k8s.io/v1beta1: v1beta1
  cluster.x-k8s.io/v1alpha4: v1alpha4
  clusterctl.cluster.x-k8s.io: ""
  clusterctl.cluster.x-k8s.io/move: ""
```

### Restoring fields in conversion
Stored version of the API object should be added as an annotation when older version of the API object is requested.
Please refer to Github issue [#355](Github issue: https://github.com/vmware/cluster-api-provider-cloud-director/issues/355) for information.

## Dev setup


## RDE management
RDEs offer persistence in VCD for otherwise CSE's K8S clusters. It contains two parts:
a. the latest desired state expressed by the user  
b. the current state of the cluster.

RDE Upgrade Steps:
1. Create new schema file under /schemas/schema_x_y_z.json
2. Locate the function ConvertToLatestRDEVersionFormat in the vcdcluster_controller.go.
3. Ensure that you invoke the correct converter function to convert the source RDE version into the latest RDE version in use.
4. Update the existing converters (and/or) add new converters to open up the upgrade paths (For example ```convertFrom100Format()```):
- Provide an automatic conversion of the content in srcCapvcdEntity.entity.status.CAPVCD content to the latest RDE version format (types.CAPVCDStatus)
- Add the placeholder for any special conversion logic inside types.CAPVCDStatus.
- Update the srcCapvcdEntity.entityType to the latest RDE version in use.
- Call the API update call to update CAPVCD entity and persist data into VCD.

Core CAPI and Clusterctl matrix
* 

Pre-release check-list
*

Post-release check-list
*



