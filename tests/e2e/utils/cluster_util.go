package utils

import (
	"bufio"
	"bytes"
	"context"
	"fmt"
	"github.com/antihax/optional"
	"github.com/vmware/cloud-provider-for-cloud-director/pkg/testingsdk"
	"github.com/vmware/cloud-provider-for-cloud-director/pkg/vcdsdk"
	swagger "github.com/vmware/cloud-provider-for-cloud-director/pkg/vcdswaggerclient_36_0"
	"github.com/vmware/cluster-api-provider-cloud-director/api/v1beta2"
	"github.com/vmware/cluster-api-provider-cloud-director/pkg/capisdk"
	rdeType "github.com/vmware/cluster-api-provider-cloud-director/pkg/vcdtypes/rde_type_1_1_0"
	"io"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	k8syaml "k8s.io/apimachinery/pkg/util/yaml"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
	"net/http"
	runtimeclient "sigs.k8s.io/controller-runtime/pkg/client"
	"strings"
)

func ValidateKubeConfig(ctx context.Context, kubeConfigBytes []byte) (*kubernetes.Clientset, error) {
	workloadRestConfig, csConfig, err := CreateClientConfig(kubeConfigBytes)
	if err != nil {
		return nil, fmt.Errorf("failed to create client configuration: %v", err)
	}

	if workloadRestConfig == nil {
		return nil, fmt.Errorf("failed to create REST configuration from kubeconfig")
	}

	if csConfig == nil {
		return nil, fmt.Errorf("failed to create clientset from REST configuration")
	}

	fmt.Println("checking pods of the cluster from all the namespaces")
	podList, err := csConfig.CoreV1().Pods("").List(ctx, metav1.ListOptions{})
	if err != nil {
		return csConfig, fmt.Errorf("failed to list pods: %v", err)
	}

	if podList == nil || len(podList.Items) == 0 {
		return csConfig, fmt.Errorf("kubeConfig is invalid as the workload cluster should have more than 1 pod")
	}

	return csConfig, nil
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

func GetVCDClusterFromCluster(ctx context.Context, client runtimeclient.Client, namespace string, name string) (*v1beta2.VCDCluster, error) {
	vcdCluster, err := getVCDCluster(ctx, client, namespace, name)
	if err != nil {
		return nil, fmt.Errorf("failed to get VCDCluster: %w", err)
	}

	if vcdCluster == nil {
		return nil, fmt.Errorf("VCDCluster is nil")
	}

	return vcdCluster, nil
}

func GetClusterNameAndNamespaceFromCAPIYaml(yamlContent []byte) (string, string, error) {
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

func GetUserOrgAndName(_ context.Context, org, username string) (string, string) {
	parts := strings.Split(username, "/")
	if len(parts) == 2 {
		return parts[0], parts[1]
	}

	return org, username
}

func ConstructCAPVCDTestClient(ctx context.Context, vcdCluster *v1beta2.VCDCluster, workloadClientSet *kubernetes.Clientset, capiYaml, clusterNameSpace, clusterName, rdeId string, getVdcClient bool) (*testingsdk.TestClient, error) {
	var refreshToken, username string
	secret, err := GetSecret(ctx, capiYaml)
	if err != nil {
		return nil, fmt.Errorf("unable to get Secret from CapiYaml: [%v]", err)
	}
	if b, exists := secret.Data["username"]; exists {
		username = strings.TrimRight(string(b), "\n")
	}
	if b, exists := secret.Data["refreshToken"]; exists {
		refreshToken = strings.TrimRight(string(b), "\n")
	}

	userOrgName, userName := GetUserOrgAndName(ctx, vcdCluster.Status.Org, username)

	testClient, err := NewTestClient(vcdCluster.Status.Site, vcdCluster.Status.Org, userOrgName, vcdCluster.Status.Ovdc, userName, refreshToken, rdeId, getVdcClient)
	if err != nil {
		return nil, fmt.Errorf("failed to construct testing client: [%v]", err)
	}
	// Using workload client as the Kubernetes client of testClient
	testClient.Cs = workloadClientSet
	return testClient, nil
}

func CreateWorkloadClusterResources(ctx context.Context, workloadClientSet *kubernetes.Clientset, capiYaml, rdeID string, vcdcluster *v1beta2.VCDCluster) error {
	var refreshToken string
	namespace := "kube-system"
	secret, err := GetSecret(ctx, capiYaml)
	if err != nil {
		return fmt.Errorf("unable to get Secret from CapiYaml: [%v]", err)
	}

	if b, exists := secret.Data["refreshToken"]; exists {
		refreshToken = strings.TrimRight(string(b), "\n")
	}

	err = CreateClusterIDSecret(ctx, workloadClientSet, namespace, rdeID)
	if err != nil {
		return fmt.Errorf("failed to create cluster ID secret: %w", err)
	}

	err = CreateAuthSecret(ctx, workloadClientSet, namespace, refreshToken)
	if err != nil {
		return fmt.Errorf("failed to create auth secret: %w", err)
	}

	host := vcdcluster.Status.Site

	org := vcdcluster.Status.Org

	clusterName := vcdcluster.Name

	ovdc := vcdcluster.Status.Ovdc

	ovdcNetwork := vcdcluster.Status.OvdcNetwork

	if host == "" {
		return fmt.Errorf("host is empty in the cluster %s", rdeID)
	}

	if org == "" {
		return fmt.Errorf("org is empty in the cluster %s", rdeID)
	}

	if clusterName == "" {
		return fmt.Errorf("clusterName is empty in the cluster %s", rdeID)
	}

	if ovdc == "" {
		return fmt.Errorf("ovdc is empty in the cluster %s", rdeID)
	}

	if ovdcNetwork == "" {
		return fmt.Errorf("ovdcNetwork is empty in the cluster %s", rdeID)
	}

	err = CreateCCMConfigMap(ctx, workloadClientSet, namespace, host, org, ovdc, rdeID, clusterName, ovdcNetwork)
	if err != nil {
		return fmt.Errorf("failed to create CCM config map: %w", err)
	}

	err = CreateCSIConfigMap(ctx, workloadClientSet, namespace, host, org, ovdc, rdeID, clusterName)
	if err != nil {
		return fmt.Errorf("failed to create CSI config map: %w", err)
	}
	return nil
}

func GetSecret(_ context.Context, capiYaml string) (*corev1.Secret, error) {
	hundredKB := 100 * 1024
	var err error = nil
	var secret *corev1.Secret

	yamlReader := k8syaml.NewYAMLReader(bufio.NewReader(bytes.NewReader([]byte(capiYaml))))

	for err == nil {
		yamlBytes, err := yamlReader.Read()
		if err == io.EOF {
			break
		}
		yamlDecoder := k8syaml.NewYAMLOrJSONDecoder(bytes.NewReader(yamlBytes), hundredKB)
		unstructuredObj := unstructured.Unstructured{}
		err = yamlDecoder.Decode(&unstructuredObj)
		if err != nil {
			return nil, fmt.Errorf("unable to parse yaml segment: [%v]\n", err)
		}

		kind := unstructuredObj.GetKind()
		name := unstructuredObj.GetName()
		switch kind {
		case SECRET:
			secret = &corev1.Secret{}
			yamlDecoder := k8syaml.NewYAMLOrJSONDecoder(bytes.NewReader(yamlBytes), hundredKB)
			if err = yamlDecoder.Decode(secret); err != nil {
				return nil, fmt.Errorf("unable to parse object of kind [%s] and name [%s]: [%v]\n",
					kind, name, err)
			}
			break
		}
	}

	err = nil
	if secret == nil {
		err = fmt.Errorf("unexpected empty secret in yaml")
	}

	return secret, err
}

func CheckRdeEntityNonExisted(ctx context.Context, client *vcdsdk.Client, rdeID, clusterName string) error {
	org, err := client.VCDClient.GetOrgByName(client.ClusterOrgName)
	if err != nil {
		return fmt.Errorf("failed to get org by name [%s]: %v", client.ClusterOrgName, err)
	}

	if org == nil || org.Org == nil {
		return fmt.Errorf("obtained nil org when getting org by name [%s]", client.ClusterOrgName)
	}

	definedEntities, resp, err := client.APIClient.DefinedEntityApi.GetDefinedEntitiesByEntityType(
		ctx,
		capisdk.CAPVCDTypeVendor,
		capisdk.CAPVCDTypeNss,
		rdeType.CapvcdRDETypeVersion,
		org.Org.ID,
		1,
		25,
		&swagger.DefinedEntityApiGetDefinedEntitiesByEntityTypeOpts{
			Filter: optional.NewString(fmt.Sprintf("id==%s", rdeID)),
		},
	)
	if err != nil {
		return fmt.Errorf("error occurred during RDE deletion; failed to fetch defined entities by entity ID [%s] for cluster [%s]: %v", rdeID, clusterName, err)
	}

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("error occurred during RDE deletion; error while fetching defined entities by entity ID [%s] for cluster [%s]", rdeID, clusterName)
	}

	if len(definedEntities.Values) > 0 {
		return fmt.Errorf("the RDE entity of cluster [%s (%s)] still exists", rdeID, clusterName)
	}

	fmt.Println("The RDE entity of cluster", clusterName, "with ID", rdeID, "has been successfully deleted.")

	return nil
}
