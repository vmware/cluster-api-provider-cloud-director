# Enable Wire Logs

Execute the following command to log HTTP requests to VCD and HTTP responses from VCD -

`kubectl set env -n capvcd-system deployment/capvcd-controller-manager GOVCD_LOG_ON_SCREEN=true -oyaml`

Once the above command is executed, CAPVCD will start logging the HTTP requests and HTTP responses made via 
go-vcloud-director SDK. The container logs can be obtained using the command `kubectl logs -n capvcd-system <CAPVCD Pod>`

To stop logging the HTTP requests and responses from VCD, the following command can be executed -

`kubectl set env -n capvcd-system deployment/capvcd-controller-manager GOVCD_LOG_ON_SCREEN-`

NOTE: Please make sure to collect the logs before and after enabling the wire log. The above commands update the CAPVCD
deployment, which creates a new CAPVCD pod. The logs present in the old pod will be lost.

Collect the logs by running this [script](https://github.com/vmware/cloud-provider-for-cloud-director/blob/main/scripts/generate-k8s-log-bundle.sh)