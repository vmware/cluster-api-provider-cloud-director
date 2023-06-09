package utils

import (
	"bufio"
	"bytes"
	"context"
	"fmt"
	"github.com/vmware/cloud-provider-for-cloud-director/pkg/testingsdk"
	"io"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/util/wait"
	k8syaml "k8s.io/apimachinery/pkg/util/yaml"
	"k8s.io/client-go/kubernetes"
	clusterv1 "sigs.k8s.io/cluster-api/api/v1beta1"
	runtimeclient "sigs.k8s.io/controller-runtime/pkg/client"
	"time"
)

func ScaleNodePool(ctx context.Context, cs *kubernetes.Clientset, desiredNodePoolSize int64, yamlContent []byte) error {
	fmt.Printf("scaling nodepool to %d\n", desiredNodePoolSize)
	var err error = nil
	yamlReader := k8syaml.NewYAMLReader(bufio.NewReader(bytes.NewReader(yamlContent)))
	hundredKB := 100 * 1024
	for err == nil {
		yamlBytes, err := yamlReader.Read()
		if err == io.EOF {
			break
		}
		yamlDecoder := k8syaml.NewYAMLOrJSONDecoder(bytes.NewReader(yamlBytes), hundredKB)
		unstructuredObj := unstructured.Unstructured{}
		err = yamlDecoder.Decode(&unstructuredObj)
		if err != nil {
			fmt.Println(err)
		}

		kind := unstructuredObj.GetKind()
		name := unstructuredObj.GetName()
		namespace := unstructuredObj.GetNamespace()
		switch kind {
		case MachineDeployment:
			emptyPayload := []patchStringValue{{
				Op:    "replace",
				Path:  "/spec/replicas",
				Value: desiredNodePoolSize,
			}}
			executedErr := patchMachineDeployment(ctx, cs, emptyPayload, name, namespace)
			if executedErr != nil {
				return executedErr
			}
			break
		}

	}
	fmt.Println("done")
	return nil

}

func MonitorK8sNodePools(
	testClient *testingsdk.TestClient, runtimeClient runtimeclient.Client, expectedWorkNodeCount int64,
) error {
	fmt.Println("monitoring k8s to confirm changes")
	timeout := timeoutMinutes * time.Minute
	pollInterval := pollIntervalSeconds * time.Second

	return wait.PollImmediate(pollInterval, timeout, func() (bool, error) {

		// getting all worker nodes
		nodeList, err := testClient.GetWorkerNodes(context.Background())
		if err != nil {
			if testingsdk.IsRetryableError(err) {
				fmt.Printf("RETRYABLE ERROR - error getting worker nodes: [%v]\n", err)
				fmt.Println("retrying monitor")
				return false, nil
			} else {
				return true, fmt.Errorf("error getting worker nodes [%v]", err)
			}
		}

		// making sure the expected number of nodes exist for each worker pool
		if len(nodeList) != int(expectedWorkNodeCount) {
			fmt.Printf("cluster does not have the right number of nodes yet\n")
			fmt.Println("retrying monitor")
			return false, nil
		}

		// waiting for all nodes to be spun up
		machineList := &clusterv1.MachineList{}
		err = runtimeClient.List(context.Background(), machineList)
		if err != nil {
			return true, fmt.Errorf("error getting machine list [%v]", err)
		}

		for _, machine := range machineList.Items {
			if machine.Status.Phase != machinePhaseRunning && machine.Status.Phase != machinePhaseProvisioned {
				fmt.Printf("machine %s : phase: %s\n", machine.Name, machine.Status.Phase)
				fmt.Println("retrying monitor")
				return false, nil
			}
		}

		fmt.Println("done checking")
		return true, nil
	})
}
