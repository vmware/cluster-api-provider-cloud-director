# Collecting CAPVCD logs

1. Download the kube config for your management cluster
2. Edit the script file `get-capvcd-logs.sh` `MANAGEMENT_KUBECONFIG="management-kubeconfig.conf"` to be the filepath of your kube config
3. Run the script to generate a log folder with the format `logs-capvcd-YEAR-MONTH-DAY-TIME`. This folder will contain logs for the management cluster as well as each workload cluster. All of these files will be compressed into a `.tar.gz` file that can be shared and uploaded

# Log requests and responses

The following command needs to be executed to log requests to VCD and responses from VCD in CAPVCD,
```shell
kubectl set env -n capvcd-system deployment/capvcd-controller-manager GOVCD_LOG_ON_SCREEN=true -oyaml
```
Once the above command is executed, the requests and responses can be found as part of the CAPVCD container logs, which can be obtained using `kubectl logs -n capvcd-system <CAPVCD_POD>` command.

To stop logging the requests and responses on to the container logs, please execute the following command.
```shell
kubectl set env -n capvcd-system deployment/capvcd-controller-manager GOVCD_LOG_ON_SCREEN-
```
