# Collecting CAPVCD logs

1. Download the kube config for your management cluster
2. Edit the script file `get-capvcd-logs.sh` `MANAGEMENT_KUBECONFIG="management-kubeconfig.conf"` to be the filepath of your kube config
3. Run the script to generate a log folder with the format `logs-capvcd-YEAR-MONTH-DAY-TIME`. This folder will contain logs for the management cluster as well as each workload cluster. All of these files will be compressed into a `.tar.gz` file that can be shared and uploaded

# Log VCD requests and responses

The following command needs to be executed to log HTTP requests to VCD and HTTP responses from VCD in CAPVCD,
```shell
kubectl set env -n capvcd-system deployment/capvcd-controller-manager GOVCD_LOG_ON_SCREEN=true -oyaml
```
Once the above command is executed, CAPVCD container will start logging HTTP requests and HTTP responses made via go-vcloud-director SDK.
The container logs can be obtained using the command - `kubectl logs -n capvcd-system <CAPVCD_POD>` .

To stop logging the HTTP requests and HTTP responses to the container logs, please execute the following command.
```shell
kubectl set env -n capvcd-system deployment/capvcd-controller-manager GOVCD_LOG_ON_SCREEN-
```
