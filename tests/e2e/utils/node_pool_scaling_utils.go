package utils

import (
	"context"
	"fmt"
	"github.com/vmware/cloud-provider-for-cloud-director/pkg/testingsdk"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/util/wait"
	clusterv1 "sigs.k8s.io/cluster-api/api/v1beta1"
	runtimeclient "sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"time"
)

func ScaleNodePool(ctx context.Context, r runtimeclient.Client, desiredNodePoolSize int64, clusterName, clusterNameSpace,
	machineDeploymentName string) error {
	fmt.Printf("Scaling node pool to %d\n", desiredNodePoolSize)

	machineDeployment := &clusterv1.MachineDeployment{}
	if err := r.Get(ctx, runtimeclient.ObjectKey{
		Namespace: clusterNameSpace,
		Name:      machineDeploymentName,
	}, machineDeployment); err != nil {
		fmt.Printf("Failed to get object: %s/%s\n", clusterNameSpace, machineDeploymentName)
		return fmt.Errorf("failed to get object: %s/%s: %w", clusterNameSpace, machineDeploymentName, err)
	}
	desiredNodePoolSizeInt32 := int32(desiredNodePoolSize)
	machineDeployment.APIVersion = "cluster.x-k8s.io/v1beta1"
	machineDeployment.Kind = "MachineDeployment"
	machineDeployment.Spec.Replicas = &desiredNodePoolSizeInt32
	machineDeployment.ManagedFields = nil
	force := true
	executedErr := r.Patch(ctx, machineDeployment, runtimeclient.Apply, &runtimeclient.PatchOptions{
		Force:        &force,
		FieldManager: FieldManager,
	})
	if executedErr != nil {
		return fmt.Errorf("failed to patch object of kind [%s] and name [%s]: %w", clusterNameSpace, machineDeploymentName, executedErr)
	}
	fmt.Println("Node pool scaling completed successfully")
	return nil
}

func MonitorK8sNodePools(testClient *testingsdk.TestClient, runtimeClient runtimeclient.Client, expectedWorkNodeCount int64) error {
	fmt.Println("Monitoring Kubernetes to confirm changes")

	timeout := timeoutMinutes * time.Minute
	pollInterval := pollIntervalSeconds * time.Second

	return wait.PollImmediate(pollInterval, timeout, func() (bool, error) {
		// Get all worker nodes
		nodeList, err := testClient.GetWorkerNodes(context.Background())
		if err != nil {
			if testingsdk.IsRetryableError(err) {
				fmt.Printf("Retryable error occurred while getting worker nodes: [%v]\n", err)
				fmt.Println("Retrying monitor")
				return false, nil
			} else {
				return true, fmt.Errorf("error getting worker nodes: [%v]", err)
			}
		}

		// Check if the expected number of nodes exist for each worker pool
		if len(nodeList) != int(expectedWorkNodeCount) {
			fmt.Println("Cluster does not have the right number of nodes yet")
			fmt.Println("Retrying monitor")
			return false, nil
		}

		// Wait for all nodes to be spun up
		machineList := &clusterv1.MachineList{}
		err = runtimeClient.List(context.Background(), machineList)
		if err != nil {
			return true, fmt.Errorf("error getting machine list: [%v]", err)
		}

		for _, machine := range machineList.Items {
			if machine.Status.Phase != machinePhaseRunning && machine.Status.Phase != machinePhaseProvisioned {
				fmt.Printf("Machine %s : Phase: %s\n", machine.Name, machine.Status.Phase)
				fmt.Println("Retrying monitor")
				return false, nil
			}
		}

		fmt.Println("Node pool monitoring completed successfully")
		return true, nil
	})
}

func WaitForClusterProvisioned(ctx context.Context, client runtimeclient.Client, clusterName, clusterNameSpace string) error {
	timeout := timeoutMinutes * time.Minute
	pollInterval := pollIntervalSeconds * time.Second
	cluster := &clusterv1.Cluster{}

	// Check the cluster readiness using wait.PollImmediate
	if err := wait.PollImmediate(pollInterval, timeout, func() (bool, error) {
		if err := client.Get(ctx, runtimeclient.ObjectKey{Namespace: clusterNameSpace, Name: clusterName}, cluster); err != nil {
			return false, fmt.Errorf("failed to get cluster: %w", err)
		}
		return cluster.Status.InfrastructureReady, nil
	}); err != nil {
		return fmt.Errorf("cluster readiness check failed: %w", err)
	}
	// Check the readiness of all machines
	machineList := &clusterv1.MachineList{}
	if err := wait.PollImmediate(pollInterval, timeout, func() (bool, error) {

		if err := client.List(ctx, machineList, runtimeclient.InNamespace(cluster.Namespace), runtimeclient.MatchingLabels{
			clusterv1.ClusterNameLabel: cluster.Name,
		}); err != nil {
			return false, fmt.Errorf("failed to list machines: %w", err)
		}
		for _, machine := range machineList.Items {
			if !controllerutil.ContainsFinalizer(&machine, clusterv1.MachineFinalizer) {
				// Machine is being deleted, so skip it
				continue
			}

			if machine.Status.Phase == machinePhaseProvisioning {
				fmt.Printf("machine %s : phase: %s\n", machine.Name, machine.Status.Phase)
				fmt.Println("retrying monitor")
				return false, nil
			}
		}
		return true, nil
	}); err != nil {
		return fmt.Errorf("machine readiness check failed: %w", err)
	}

	return nil
}

func WaitForMachinesRunning(ctx context.Context, client runtimeclient.Client, clusterName, clusterNameSpace string) error {
	timeout := timeoutMinutes * time.Minute
	pollInterval := pollIntervalSeconds * time.Second
	machineList := &clusterv1.MachineList{}
	if err := wait.PollImmediate(pollInterval, timeout, func() (bool, error) {
		if err := client.List(ctx, machineList, runtimeclient.InNamespace(clusterNameSpace), runtimeclient.MatchingLabels{
			clusterv1.ClusterNameLabel: clusterName,
		}); err != nil {
			return false, fmt.Errorf("failed to list machines: %w", err)
		}
		for _, machine := range machineList.Items {
			if !controllerutil.ContainsFinalizer(&machine, clusterv1.MachineFinalizer) {
				// Machine is being deleted, so skip it
				continue
			}

			if machine.Status.Phase != machinePhaseRunning {
				fmt.Printf("machine %s : phase: %s\n", machine.Name, machine.Status.Phase)
				fmt.Println("retrying monitor")
				return false, nil
			}
		}
		return true, nil
	}); err != nil {
		return fmt.Errorf("machine readiness check failed: %w", err)
	}

	return nil
}

func WaitForClusterDelete(ctx context.Context, client runtimeclient.Client, clusterName, clusterNameSpace string) error {
	timeout := timeoutMinutes * time.Minute
	pollInterval := pollIntervalSeconds * time.Second
	cluster := &clusterv1.Cluster{}
	// Check the cluster deletion using wait.PollImmediate
	if err := wait.PollImmediate(pollInterval, timeout, func() (bool, error) {
		err := client.Get(ctx, runtimeclient.ObjectKey{Namespace: clusterNameSpace, Name: clusterName}, cluster)
		if err != nil && errors.IsNotFound(err) {
			// Cluster is deleted
			return true, nil
		}
		return false, err
	}); err != nil {
		return fmt.Errorf("cluster deletion check failed: %w", err)
	}

	return nil
}
