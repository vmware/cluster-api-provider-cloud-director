# Collecting CAPVCD logs

1. Download the kube config for your management cluster
2. Edit the script file `get-capvcd-logs.sh` `MANAGEMENT_KUBECONFIG="management-kubeconfig.conf"` to be the filepath of your kube config
3. Run the script to generate a log folder with the format `logs-capvcd-YEAR-MONTH-DAY-TIME`. This folder will contain logs for the management cluster as well as each workload cluster. All of these files will be compressed into a `.tar.gz` file that can be shared and uploaded
