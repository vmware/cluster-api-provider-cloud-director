# Machine Health Check
Machine Health Check can be configured and deployed to automatically repair damaged machines. This can be achieved by creating a MachineHealthCheck API object.

The following shows an example of a MachineHealthCheck API object.
```shell
apiVersion: cluster.x-k8s.io/v1beta1
kind: MachineHealthCheck
metadata:
  name: capi-quickstart-node-unhealthy-5m
spec:
  # clusterName is required to associate this MachineHealthCheck with a particular cluster
  clusterName: capi-quickstart
  # (Optional) maxUnhealthy prevents further remediation if the cluster is already partially unhealthy
  maxUnhealthy: 40%
  # (Optional) nodeStartupTimeout determines how long a MachineHealthCheck should wait for
  # a Node to join the cluster, before considering a Machine unhealthy.
  # Defaults to 10 minutes if not specified.
  # Set to 0 to disable the node startup timeout.
  # Disabling this timeout will prevent a Machine from being considered unhealthy when
  # the Node it created has not yet registered with the cluster. This can be useful when
  # Nodes take a long time to start up or when you only want condition based checks for
  # Machine health.
  nodeStartupTimeout: 10m
  # selector is used to determine which Machines should be health checked
  selector:
    matchLabels:
      nodepool: nodepool-0
  # Conditions to check on Nodes for matched Machines, if any condition is matched for the duration of its timeout, the Machine is considered unhealthy
  unhealthyConditions:
  - type: Ready
    status: Unknown
    timeout: 300s
  - type: Ready
    status: "False"
    timeout: 300s
```

Please refer to Cluster API book's section about [Healthchecking](https://cluster-api.sigs.k8s.io/tasks/automated-machine-management/healthchecking.html) to learn more about Machine Health Check.

**Note:**
When MachineHealthCheck is not configured in a cluster, and if a cluster node running StatefulSet goes down, the StatefulSet pods will be stuck in "Terminating" phase. StatefulSet pods won't be scheduled to a different node.
However, if MachineHealthCheck is configured on the cluster, Cluster API will eventually ignore the non terminated pods and re-creates the failed Machine. The StatefulSet pods will start executing on the new Machine.
