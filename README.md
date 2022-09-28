# Kubernetes Cluster API Provider Cloud Director

## Overview
The Cluster API brings declarative, Kubernetes-style APIs to cluster creation, configuration and management. Cluster API Provider for Cloud Director is a concrete implementation of Cluster API for VMware Cloud Director.

## Quick start
Check out our [Cluster API quick start guide](docs/QUICKSTART.md) to create a Kubernetes cluster on VMware Cloud Director using Cluster API.

<a name="support_matrix"></a>
## Support Policy
The version of Cluster API Provider Cloud Director and Installation that are compatible for a given CAPVCD container image are described in the following compatibility matrix:

|                                  CAPVCD Version                                   | VMware Cloud Director API | VMware Cloud Director Installation | clusterctl CLI version | Kubernetes Versions |
|:---------------------------------------------------------------------------------:| :-----------------------: | :--------------------------------: | :---: | :------------------ |
| [0.5.0](https://github.com/vmware/cluster-api-provider-cloud-director/tree/0.5.0) | 36.0+ | 10.3.1+ <br/>(10.3.1 needs hot-patch to prevent VCD cell crashes in multi-cell environments) | [0.4.7](https://github.com/kubernetes-sigs/cluster-api/releases/tag/v0.4.7) |<ul><li>1.21</li><li>1.20</li></ul>|
| [0.5.1](https://github.com/vmware/cluster-api-provider-cloud-director/tree/0.5.1) | 36.0+ | 10.3.1+ <br/>(10.3.1 needs hot-patch to prevent VCD cell crashes in multi-cell environments) | [0.4.7](https://github.com/kubernetes-sigs/cluster-api/releases/tag/v0.4.7) |<ul><li>1.21</li><li>1.20</li></ul>|
|  [main](https://github.com/vmware/cluster-api-provider-cloud-director/tree/main)  | 36.0+ | 10.3.1+ <br/>(10.3.1 needs hot-patch to prevent VCD cell crashes in multi-cell environments) | [1.1.3](https://github.com/kubernetes-sigs/cluster-api/releases/tag/v1.1.3) |<ul><li>1.21</li><li>1.20</li></ul>|

Cluster API versions:

|                          | v1alpha4 (v1.0) | v1beta1 (v1.1) |
|--------------------------| --------------  |----------------|
| CAPVCD v1alpha4 (v0.5)   |     ✓           | Not supported  |
| CAPVCD v1beta1 (main)    |      ✓           | ✓              |

Kubernetes versions:

|                        | v1.20 | v1.21 | v1.22 |
| -----------------------| ----- | ----- | ----- |
| CAPVCD v1alpha4 (v0.5) | ✓     | ✓     |       |

## Troubleshooting
[Collect CAPI log bundle for Cloud Director](https://github.com/vmware/cluster-api-provider-cloud-director/tree/main/scripts)

## Contributing
The cluster-api-provider-cloud-director project team welcomes contributions from the community. Before you start working with cluster-api-provider-cloud-director, please refer to [CONTRIBUTING.md](CONTRIBUTING.md).

## License
[Apache-2.0](LICENSE)
