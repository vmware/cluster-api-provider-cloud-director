# Contributing Guidelines

The **cluster-api-provider-cloud-director** project team welcomes contributions from the community. If you wish to contribute code and you have not signed our contributor license agreement ([CLA](https://cla.vmware.com/cla/1/preview)), our bot will update the issue when you open a Pull Request. For any questions about the CLA process, please refer to our [FAQ](https://cla.vmware.com/faq)

## Sign the Contributor License Agreement

We'd love to accept your patches! Before you start working with cluster-api-provider-cloud-director, please read our [Developer Certificate of Origin](https://cla.vmware.com/dco). All contributions to this repository must be signed as described on that page. Your signature certifies that you wrote the patch or have the right to pass it on as an open-source patch.

## Reporting an issue

If you find a bug or a feature request related to cloud-provider-for-cloud-director you can create a new GitHub issue in this repo.

## Development Environment

1. Install GoLang 1.16.x and set up your dev environment with `GOPATH`
2. Check out code into `$GOPATH/github.com/vmware/cluster-api-provider-cloud-director`

## Building container image for the Cloud Provider

1. Ensure that you have `docker` installed in your dev/build machine.
2. Run `REGISTRY=<registry where you want to push your images> make all`

## Contributing a Patch

1. Submit an issue describing your proposed change to the repo.
2. Fork the **cluster-api-provider-cloud-director** repo, develop and test your code changes.
3. Submit a pull request.

## Contact

Please use Github Pull Requests and Github Issues as a means to start a conversation with the team.
