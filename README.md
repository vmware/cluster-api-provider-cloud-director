# Kubernetes Cluster API Provider Cloud Director

## Overview
The Cluster API brings declarative, Kubernetes-style APIs to cluster creation, configuration and management. Cluster API Provider for Cloud Director is a concrete implementation of Cluster API for VMware Cloud Director.

## Quick start
Check out our [Cluster API quick start guide](docs/QUICKSTART.md) to create a Kubernetes cluster on VMware Cloud Director using Cluster API.

<a name="support_matrix"></a>
## Support Policy
The version of Cluster API Provider Cloud Director and Installation that are compatible for a given CAPVCD container image are described in the following compatibility matrix:

Note: The Kubernetes versions mentioned are what we have qualified with.

|                                  CAPVCD Version                                   | VMware Cloud Director API |                              VMware Cloud Director Installation                              |                                                                CoreCAPI/Clusterctl CLI version                                                                 | Kubernetes Versions                                                                     | CSI Versions                                                                                                                                                                                                                                                                                                                                                                                                        | CPI Versions                                                                                                                                                                                                                                                                                              | 
|:---------------------------------------------------------------------------------:|:-------------------------:|:--------------------------------------------------------------------------------------------:|:--------------------------------------------------------------------------------------------------------------------------------------------------------------:|:----------------------------------------------------------------------------------------|:--------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------|:----------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------|
|  [main](https://github.com/vmware/cluster-api-provider-cloud-director/tree/main)  |           36.0+           | 10.3.1+ <br/>(10.3.1 needs hot-patch to prevent VCD cell crashes in multi-cell environments) |                                          [1.4.0](https://github.com/kubernetes-sigs/cluster-api/releases/tag/v1.4.0)                                           | <ul><li>1.25</li><li>1.24</li><li>1.23</li><li>1.22</li><li>1.21</li><li>1.20</li></ul> | <ul><li>[1.4.0](https://github.com/vmware/cloud-director-named-disk-csi-driver/releases/tag/1.4)</li><li>[1.3.2](https://github.com/vmware/cloud-director-named-disk-csi-driver/releases/tag/1.3.2)</li><li>[1.3.1](https://github.com/vmware/cloud-director-named-disk-csi-driver/releases/tag/1.3.1)</li><li>[1.3.0](https://github.com/vmware/cloud-director-named-disk-csi-driver/releases/tag/1.3.0)</li></ul> | <ul><li>[1.4.0](https://github.com/vmware/cloud-provider-for-cloud-director/releases/tag/1.4.0)</li><li>[1.3.0](https://github.com/vmware/cloud-provider-for-cloud-director/releases/tag/1.3.0)</li><li>[1.2.0](https://github.com/vmware/cloud-provider-for-cloud-director/releases/tag/1.2.0)</li></ul> |
| [1.0.2](https://github.com/vmware/cluster-api-provider-cloud-director/tree/1.0.2) |           36.0+           | 10.3.1+ <br/>(10.3.1 needs hot-patch to prevent VCD cell crashes in multi-cell environments) |                                          [1.1.3](https://github.com/kubernetes-sigs/cluster-api/releases/tag/v1.1.3)                                           | <ul><li>1.24</li><li>1.23</li><li>1.22</li><li>1.21</li><li>1.20</li></ul>              | <ul><li>[1.3.1](https://github.com/vmware/cloud-director-named-disk-csi-driver/releases/tag/1.3.1)</li><li>[1.3.0](https://github.com/vmware/cloud-director-named-disk-csi-driver/releases/tag/1.3.0)</li></ul>                                                                                                                                                                                                     | <ul><li>[1.3.0](https://github.com/vmware/cloud-provider-for-cloud-director/releases/tag/1.3.0)</li><li>[1.2.0](https://github.com/vmware/cloud-provider-for-cloud-director/releases/tag/1.2.0)</li></ul>                                                                                                 |
| [1.0.1](https://github.com/vmware/cluster-api-provider-cloud-director/tree/1.0.1) |           36.0+           | 10.3.1+ <br/>(10.3.1 needs hot-patch to prevent VCD cell crashes in multi-cell environments) |                                          [1.1.3](https://github.com/kubernetes-sigs/cluster-api/releases/tag/v1.1.3)                                           | <ul><li>1.24</li><li>1.23</li><li>1.22</li><li>1.21</li><li>1.20</li></ul>              | <ul><li>[1.3.1](https://github.com/vmware/cloud-director-named-disk-csi-driver/releases/tag/1.3.1)</li><li>[1.3.0](https://github.com/vmware/cloud-director-named-disk-csi-driver/releases/tag/1.3.0)</li></ul>                                                                                                                                                                                                     | <ul><li>[1.3.0](https://github.com/vmware/cloud-provider-for-cloud-director/releases/tag/1.3.0)</li><li>[1.2.0](https://github.com/vmware/cloud-provider-for-cloud-director/releases/tag/1.2.0)</li></ul>                                                                                                 |
| [1.0.0](https://github.com/vmware/cluster-api-provider-cloud-director/tree/1.0.0) |           36.0+           | 10.3.1+ <br/>(10.3.1 needs hot-patch to prevent VCD cell crashes in multi-cell environments) |                                          [1.1.3](https://github.com/kubernetes-sigs/cluster-api/releases/tag/v1.1.3)                                           | <ul><li>1.22</li><li>1.21</li><li>1.20</li></ul>                                        | <ul><li>[1.3.1](https://github.com/vmware/cloud-director-named-disk-csi-driver/releases/tag/1.3.1)</li><li>[1.3.0](https://github.com/vmware/cloud-director-named-disk-csi-driver/releases/tag/1.3.0)</li></ul>                                                                                                                                                                                                     | <ul><li>[1.3.0](https://github.com/vmware/cloud-provider-for-cloud-director/releases/tag/1.3.0)</li><li>[1.2.0](https://github.com/vmware/cloud-provider-for-cloud-director/releases/tag/1.2.0)</li></ul>                                                                                                 |
| [0.5.1](https://github.com/vmware/cluster-api-provider-cloud-director/tree/0.5.1) |           36.0+           | 10.3.1+ <br/>(10.3.1 needs hot-patch to prevent VCD cell crashes in multi-cell environments) |                                          [0.4.7](https://github.com/kubernetes-sigs/cluster-api/releases/tag/v0.4.7)                                           | <ul><li>1.21</li><li>1.20</li></ul>                                                     |
| [0.5.0](https://github.com/vmware/cluster-api-provider-cloud-director/tree/0.5.0) |           36.0+           | 10.3.1+ <br/>(10.3.1 needs hot-patch to prevent VCD cell crashes in multi-cell environments) |                                          [0.4.7](https://github.com/kubernetes-sigs/cluster-api/releases/tag/v0.4.7)                                           | <ul><li>1.21</li><li>1.20</li></ul>                                                     |

Cluster API versions:

|                         | v1alpha4 (v1.0) | v1beta1 (v1.1) |
|-------------------------| --------------  |----------------|
| CAPVCD v1beta1 (main)   |     ✓           | ✓              |
| CAPVCD v1beta1 (v1.0)   |     ✓           | ✓              |
| CAPVCD v1alpha4 (v0.5.x) |     ✓           | Not supported  |

TKG versions:

|                          | TKG versions                         |
|--------------------------|--------------------------------------| 
| CAPVCD v1beta1  (main)   | 2.2.0*, 2.1.1*, 1.6.1*, 1.5.4, 1.4.3 |
| CAPVCD v1beta1  (v1.1.0) | 2.2.0, 2.1.1, 1.6.1, 1.5.4, 1.4.3    |
| CAPVCD v1beta1  (v1.0.2) | 1.6.1, 1.5.4, 1.4.3                  |
| CAPVCD v1beta1  (v1.0.1) | 1.6.1, 1.5.4, 1.4.3                  |
| CAPVCD v1beta1  (v1.0.0) | 1.5.4, 1.4.3                         | 
| CAPVCD v1alpha4 (v0.5.x) | 1.4.0, 1.3.1                         |

TKG versions 2.2.0, 2.1.1, 1.6.1*: CAPVCD needs to be upgraded to at least 1.0.2 to support workload cluster creations 
greater than or equal to TKG 1.6.1. This is also applicable for upgrading existing clusters (of TKG versions less than or 
equal to 1.5.4) to TKG versions greater than or equal to 1.6.1

## Troubleshooting
[Collect CAPI log bundle for Cloud Director](https://github.com/vmware/cluster-api-provider-cloud-director/tree/main/scripts).

Refer to [enable wire logs for CAPVCD](docs/WIRE_LOGS.md) to log HTTP requests/responses between CAPVCD and Cloud Director

## Contributing
The cluster-api-provider-cloud-director project team welcomes contributions from the community. Before you start working with cluster-api-provider-cloud-director, please refer to [CONTRIBUTING.md](CONTRIBUTING.md).

## Communicating with the maintainers
[#cluster-api-cloud-director](https://kubernetes.slack.com/messages/C04JFT7GDGR) on Kubernetes slack can be used to communicate with the maintainers to learn more about cluster-api for Cloud Director or to discuss any potential issues.

## License
[Apache-2.0](LICENSE)
