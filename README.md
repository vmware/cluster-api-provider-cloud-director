# Kubernetes Cluster API Provider Cloud Director

## Overview
The Cluster API brings declarative, Kubernetes-style APIs to cluster creation, configuration and management. Cluster API Provider for Cloud Director is a concrete implementation of Cluster API for VMware Cloud Director.

## Quick start
Check out [CAPVCD quick start guide](docs/QUICKSTART.md) to create a Kubernetes cluster on vCloud Director using Cluster API

## Compatibility with Cluster API, CAPVCD, and Kubernetes Versions

Support matrix between CAPVCD and Cluster API versions:

|                        | v1alpha3 (v0.3) | v1alpha4 (v0.4) | v1beta1 (v1.0) |
| -----------------------| --------------- | --------------- | -------------- |
| CAPVCD v1alpha4 (v0.5) |                 | ✓               |                |

Support matrix between CAPVCD and Kubernetes versions:

|                        | v1.20 | v1.21 | v1.22 |
| -----------------------| ----- | ----- | ----- |
| CAPVCD v1alpha4 (v0.5) | ✓     | ✓     |       |

## Known Issues
[KNOWN ISSUES](KNOWNISSUES.md)

## FAQ
[FAQ](docs/FAQ.md)


## Contributing

The cluster-api-provider-cloud-director project team welcomes contributions from the community. Before you start working with cluster-api-provider-cloud-director, please refer to [CONTRIBUTING.md](CONTRIBUTING.md).

## License
[Apache-2.0](LICENSE)

