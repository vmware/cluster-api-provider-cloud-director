package utils

import (
	"bufio"
	"bytes"
	"context"
	"fmt"
	"io"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	k8syaml "k8s.io/apimachinery/pkg/util/yaml"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
)

func ValidateKubeConfig(ctx context.Context, kubeConfigBytes []byte) error {
	workloadRestConfig, err := clientcmd.RESTConfigFromKubeConfig(kubeConfigBytes)
	workloadRestConfig, csConfig, err := CreateClientConfig(kubeConfigBytes)
	if err != nil {
		return fmt.Errorf("failed to create client configuration: %w", err)
	}

	if workloadRestConfig == nil {
		return fmt.Errorf("failed to create REST configuration from kubeconfig")
	}

	if csConfig == nil {
		return fmt.Errorf("failed to create clientset from REST configuration")
	}

	podList, err := csConfig.CoreV1().Pods("").List(ctx, metav1.ListOptions{})
	if err != nil {
		return fmt.Errorf("failed to list pods: %w", err)
	}

	if podList == nil || len(podList.Items) == 0 {
		return fmt.Errorf("kubeConfig is invalid as the workload cluster should have more than 1 pod")
	}

	return nil
}

func CreateClientConfig(kubeConfigBytes []byte) (*rest.Config, *kubernetes.Clientset, error) {
	workloadRestConfig, err := clientcmd.RESTConfigFromKubeConfig(kubeConfigBytes)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to create REST config from kubeconfig: %w", err)
	}

	csConfig, err := kubernetes.NewForConfig(workloadRestConfig)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to create clientset from REST config: %w", err)
	}

	return workloadRestConfig, csConfig, nil
}

func GetRDEIdFromVcdCluster(ctx context.Context, cs *kubernetes.Clientset, namespace string, name string) (string, error) {
	vcdCluster, err := getVCDCluster(ctx, cs, namespace, name)
	if err != nil {
		return "", fmt.Errorf("failed to get VCDCluster: %w", err)
	}

	if vcdCluster == nil {
		return "", fmt.Errorf("VCDCluster is nil")
	}

	return vcdCluster.Spec.RDEId, nil
}

func GetClusterNameANDNamespaceFromCAPIYaml(yamlContent []byte) (string, string, error) {
	var err error
	var resourceName string
	var namespace string
	yamlReader := k8syaml.NewYAMLReader(bufio.NewReader(bytes.NewReader(yamlContent)))
	hundredKB := 100 * 1024
	for err == nil {
		yamlBytes, readErr := yamlReader.Read()
		if readErr == io.EOF {
			break
		}
		yamlDecoder := k8syaml.NewYAMLOrJSONDecoder(bytes.NewReader(yamlBytes), hundredKB)
		unstructuredObj := unstructured.Unstructured{}
		err = yamlDecoder.Decode(&unstructuredObj)
		if err != nil {
			return "", "", fmt.Errorf("failed to decode YAML object: %w", err)
		}

		kind := unstructuredObj.GetKind()
		resourceName = unstructuredObj.GetName()
		namespace = unstructuredObj.GetNamespace()
		switch kind {
		case Cluster:
			if resourceName == "" || namespace == "" {
				return resourceName, namespace, fmt.Errorf("please examine the format of capi yaml. The clustername and namespace should not be empty")
			}
			return resourceName, namespace, nil
		}

		if err != nil {
			return resourceName, namespace, fmt.Errorf("failed to get clustername and namespace: %v", err)
		}
	}
	return resourceName, namespace, nil
}
