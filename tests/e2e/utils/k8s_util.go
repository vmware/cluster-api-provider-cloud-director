package utils

import (
	"bufio"
	"bytes"
	"context"
	_ "embed"
	"encoding/json"
	"fmt"
	infrav2 "github.com/vmware/cluster-api-provider-cloud-director/api/v1beta2"
	"io"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	apierrs "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	k8stypes "k8s.io/apimachinery/pkg/types"
	k8syaml "k8s.io/apimachinery/pkg/util/yaml"
	"k8s.io/client-go/kubernetes"
	clusterv1 "sigs.k8s.io/cluster-api/api/v1beta1"
	bootstrapv1 "sigs.k8s.io/cluster-api/bootstrap/kubeadm/api/v1beta1"
	kcpv1 "sigs.k8s.io/cluster-api/controlplane/kubeadm/api/v1beta1"
	"text/template"
)

const (
	CAPIObjectVersion   = "v1beta1"
	CAPVCDObjectVersion = "v1beta2"
)

//go:embed config_map_files/vcloud-ccm-config.yaml
var ccmConfigMapYamlTemplate string

//go:embed config_map_files/vcloud-csi-config.yaml
var csiConfigMapYamlTemplate string

func ApplyCAPIYaml(ctx context.Context, cs *kubernetes.Clientset, yamlContent []byte) (*clusterv1.Cluster, error) {
	yamlReader := k8syaml.NewYAMLReader(bufio.NewReader(bytes.NewReader(yamlContent)))
	hundredKB := 100 * 1024

	var cluster *clusterv1.Cluster

	for {
		yamlBytes, err := yamlReader.Read()
		if err == io.EOF {
			break
		} else if err != nil {
			return nil, err
		}

		yamlDecoder := k8syaml.NewYAMLOrJSONDecoder(bytes.NewReader(yamlBytes), hundredKB)
		unstructuredObj := unstructured.Unstructured{}
		if err = yamlDecoder.Decode(&unstructuredObj); err != nil {
			fmt.Println(err)
			continue
		}
		kind := unstructuredObj.GetKind()
		namespace := unstructuredObj.GetNamespace()

		switch kind {
		case "Cluster":
			yamlDecoder := k8syaml.NewYAMLOrJSONDecoder(bytes.NewReader(yamlBytes), hundredKB)
			if err = yamlDecoder.Decode(&cluster); err != nil {
				fmt.Println(err)
				continue
			}
			err = createCluster(ctx, cs, cluster, namespace)
		case "VCDCluster":
			vcdCluster := &infrav2.VCDCluster{}
			yamlDecoder := k8syaml.NewYAMLOrJSONDecoder(bytes.NewReader(yamlBytes), hundredKB)
			if err = yamlDecoder.Decode(&vcdCluster); err != nil {
				fmt.Println(err)
				continue
			}
			err = createVCDCluster(ctx, cs, vcdCluster, namespace)
		case "Secret":
			secret := &corev1.Secret{}
			yamlDecoder := k8syaml.NewYAMLOrJSONDecoder(bytes.NewReader(yamlBytes), hundredKB)
			if err = yamlDecoder.Decode(&secret); err != nil {
				fmt.Println(err)
				continue
			}
			err = createSecret(ctx, cs, secret, namespace)
		case "VCDMachineTemplate":
			vcdMachineTemplate := &infrav2.VCDMachineTemplate{}
			yamlDecoder := k8syaml.NewYAMLOrJSONDecoder(bytes.NewReader(yamlBytes), hundredKB)
			if err = yamlDecoder.Decode(&vcdMachineTemplate); err != nil {
				fmt.Println(err)
				continue
			}
			err = createVCDMachineTemplate(ctx, cs, vcdMachineTemplate, namespace)
		case "KubeadmControlPlane":
			kcp := &kcpv1.KubeadmControlPlane{}
			yamlDecoder := k8syaml.NewYAMLOrJSONDecoder(bytes.NewReader(yamlBytes), hundredKB)
			if err = yamlDecoder.Decode(&kcp); err != nil {
				fmt.Println(err)
				continue
			}
			err = createKubeadmControlPlane(ctx, cs, kcp, namespace)
		case "KubeadmConfigTemplate":
			kubeadmConfig := &bootstrapv1.KubeadmConfigTemplate{}
			yamlDecoder := k8syaml.NewYAMLOrJSONDecoder(bytes.NewReader(yamlBytes), hundredKB)
			if err = yamlDecoder.Decode(&kubeadmConfig); err != nil {
				fmt.Println(err)
				continue
			}
			err = createKubeadmConfigTemplate(ctx, cs, kubeadmConfig, namespace)
		case "MachineDeployment":
			machineDeployment := &clusterv1.MachineDeployment{}
			yamlDecoder := k8syaml.NewYAMLOrJSONDecoder(bytes.NewReader(yamlBytes), hundredKB)
			if err = yamlDecoder.Decode(&machineDeployment); err != nil {
				fmt.Println(err)
				continue
			}
			err = createMachineDeployment(ctx, cs, machineDeployment, namespace)
		default:
			fmt.Printf("Unsupported kind: %s\n", kind)
		}

		if err != nil {
			return cluster, err
		}
	}

	return cluster, nil
}

func DeleteCAPIYaml(ctx context.Context, cs *kubernetes.Clientset, yamlContent []byte) error {
	var err error
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
			return fmt.Errorf("failed to decode YAML object: %w", err)
		}

		kind := unstructuredObj.GetKind()
		name := unstructuredObj.GetName()
		ns := unstructuredObj.GetNamespace()
		switch kind {
		case Cluster:
			err = deleteCluster(ctx, cs, ns, name)
		case VCDCluster:
			err = deleteVCDCluster(ctx, cs, ns, name)
		case VCDMachineTemplate:
			err = deleteVCDMachineTemplate(ctx, cs, ns, name)
		case KubeadmControlPlane:
			err = deleteKubeadmControlPlane(ctx, cs, ns, name)
		case KubeadmConfigTemplate:
			err = deleteKubeadmConfigTemplate(ctx, cs, ns, name)
		case MachineDeployment:
			err = deleteMachineDeployment(ctx, cs, ns, name)
		}

		if err != nil {
			return fmt.Errorf("failed to delete resource [%s]: %v", kind, err)
		}
	}
	return nil
}

func patchMachineDeployment(ctx context.Context, cs *kubernetes.Clientset, patchPayload []patchStringValue, resourceName string, namespace string) error {

	emptyPayloadBytes, _ := json.Marshal(patchPayload)

	_, err := cs.RESTClient().
		Patch(k8stypes.JSONPatchType).
		AbsPath(fmt.Sprintf("/apis/cluster.x-k8s.io/%s", CAPIObjectVersion)).Namespace(namespace).Resource("machinedeployments").Name(resourceName).
		Body(emptyPayloadBytes).
		DoRaw(ctx)

	if err != nil {
		return fmt.Errorf("failed to patch machine deployment: %v", err)
	}
	return nil
}

func CreateOrGetNameSpace(ctx context.Context, cs *kubernetes.Clientset, namespace string) error {
	_, err := cs.CoreV1().Namespaces().Get(ctx, namespace, metav1.GetOptions{})
	if err != nil {
		if apierrs.IsNotFound(err) {
			nsInstance := &corev1.Namespace{
				ObjectMeta: metav1.ObjectMeta{
					Name: namespace,
				},
			}
			_, createErr := cs.CoreV1().Namespaces().Create(ctx, nsInstance, metav1.CreateOptions{})
			if createErr != nil {
				return fmt.Errorf("error occurred when creating the namespace [%s]: [%v]", namespace, createErr)
			}
			return nil
		}
		return fmt.Errorf("error occurred when getting the namespace [%s]: [%v]", namespace, err)
	}
	return nil
}

func CreateAuthSecret(ctx context.Context, cs *kubernetes.Clientset, namespace string, refreshToken string) error {
	secretName := "vcloud-basic-auth"
	username := ""
	password := ""

	// Create the secret object
	secret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      secretName,
			Namespace: namespace,
		},
		StringData: map[string]string{
			"refreshToken": refreshToken,
			"username":     username,
			"password":     password,
		},
	}
	return createSecret(ctx, cs, secret, namespace)
}

func CreateClusterIDSecret(ctx context.Context, cs *kubernetes.Clientset, namespace string, clusterID string) error {
	secretName := "vcloud-clusterid-secret"
	secret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      secretName,
			Namespace: namespace,
		},
		StringData: map[string]string{
			"clusterid": clusterID,
		},
	}
	return createSecret(ctx, cs, secret, namespace)
}

func createCluster(ctx context.Context, cs *kubernetes.Clientset, cluster *clusterv1.Cluster, namespace string) error {
	body, err := json.Marshal(cluster)
	if err != nil {
		return err
	}
	_, err = cs.RESTClient().
		Post().
		AbsPath(fmt.Sprintf("/apis/cluster.x-k8s.io/%s", CAPIObjectVersion)).
		Resource("clusters").
		Namespace(namespace).
		Body(body).
		DoRaw(ctx)
	if err != nil {
		return fmt.Errorf("failed to create cluster: %v", err)
	}
	return nil
}

func deleteCluster(ctx context.Context, cs *kubernetes.Clientset, namespace string, name string) error {
	_, err := getCluster(ctx, cs, namespace, name)
	if err != nil {
		if errors.IsNotFound(err) {
			fmt.Printf("Did not find the cluster resource: %s, skipping the delete operation\n", name)
			return nil
		}
		return fmt.Errorf("failed to get cluster: %w", err)
	}

	err = cs.RESTClient().Delete().
		AbsPath(fmt.Sprintf("/apis/cluster.x-k8s.io/%s", CAPIObjectVersion)).
		Resource("clusters").
		Namespace(namespace).
		Name(name).
		Do(ctx).
		Error()

	if err != nil {
		return fmt.Errorf("failed to delete cluster: %w", err)
	}

	return nil
}

func CreateCCMConfigMap(ctx context.Context, cs *kubernetes.Clientset, namespace, host, org, vdcName, clusterID, clusterName, ovdcNetwork string) error {
	var err error
	hundredKB := 100 * 1024
	cpiConfigMapTemplate := template.New("cpi_config_map")

	if cpiConfigMapTemplate, err = cpiConfigMapTemplate.Parse(ccmConfigMapYamlTemplate); err != nil {
		return fmt.Errorf("failed to parse cpi_config_map template: %v", err)
	}
	configMapInput := ConfigMapInput{
		VcdHost:   host,
		OVDC:      vdcName,
		ClusterID: clusterID,
		ORG:       org,
		Network:   ovdcNetwork,
		VAPP:      clusterName,
	}
	buff := bytes.Buffer{}

	if err = cpiConfigMapTemplate.Execute(&buff, configMapInput); err != nil {
		return fmt.Errorf("failed to render cpi_config_map template: %v", err)
	}
	configMap := &corev1.ConfigMap{}
	yamlDecoder := k8syaml.NewYAMLOrJSONDecoder(bytes.NewReader(buff.Bytes()), hundredKB)
	if err = yamlDecoder.Decode(configMap); err != nil {
		return fmt.Errorf("failed to decode ConfigMap: %v", err)
	}

	return createConfigMap(ctx, cs, configMap, namespace)
}

func CreateCSIConfigMap(ctx context.Context, cs *kubernetes.Clientset, namespace, host, org, vdcName, clusterID, vAppName string) error {
	hundredKB := 100 * 1024
	var err error
	configMap := &corev1.ConfigMap{}
	buff := bytes.Buffer{}

	configMapInput := ConfigMapInput{
		VcdHost:   host,
		OVDC:      vdcName,
		ClusterID: clusterID,
		ORG:       org,
		VAPP:      vAppName,
	}

	csiConfigMapTemplate := template.New("csi_config_map")

	if csiConfigMapTemplate, err = csiConfigMapTemplate.Parse(csiConfigMapYamlTemplate); err != nil {
		return fmt.Errorf("failed to parse csi_config_map template: %v", err)
	}
	if err = csiConfigMapTemplate.Execute(&buff, configMapInput); err != nil {
		return fmt.Errorf("failed to render csi_config_map template: %v", err)
	}

	yamlDecoder := k8syaml.NewYAMLOrJSONDecoder(bytes.NewReader(buff.Bytes()), hundredKB)
	if err = yamlDecoder.Decode(configMap); err != nil {
		return fmt.Errorf("failed to decode ConfigMap: %v", err)
	}
	return createConfigMap(ctx, cs, configMap, namespace)
}

func getCluster(ctx context.Context, cs *kubernetes.Clientset, namespace string, name string) (*clusterv1.Cluster, error) {
	hundredKB := 100 * 1024
	cluster := &clusterv1.Cluster{}
	resp, err := cs.RESTClient().Get().AbsPath(fmt.Sprintf("/apis/cluster.x-k8s.io/%s", CAPIObjectVersion)).
		Resource("clusters").
		Namespace(namespace).
		Name(name).
		DoRaw(ctx)

	errors.IsNotFound(err)
	if err != nil {
		if errors.IsNotFound(err) {
			return nil, err
		}
		return nil, fmt.Errorf("failed to get cluster: %v", err)
	}
	yamlDecoder := k8syaml.NewYAMLOrJSONDecoder(bytes.NewReader(resp), hundredKB)
	if err = yamlDecoder.Decode(cluster); err != nil {
		return cluster, fmt.Errorf("error occurred when decoding cluster: [%v]", err)
	}
	return cluster, nil
}

func createVCDCluster(ctx context.Context, cs *kubernetes.Clientset, vcdCluster *infrav2.VCDCluster, namespace string) error {

	body, err := json.Marshal(vcdCluster)
	if err != nil {
		return err
	}

	_, err = cs.RESTClient().
		Post().
		AbsPath(fmt.Sprintf("/apis/infrastructure.cluster.x-k8s.io/%s", CAPVCDObjectVersion)).
		Resource("vcdclusters").
		Namespace(namespace).
		Body(body).
		DoRaw(ctx)

	if err != nil {
		return fmt.Errorf("failed to create vcdcluster: %v", err)
	}
	return nil
}

func deleteVCDCluster(ctx context.Context, cs *kubernetes.Clientset, namespace string, name string) error {
	_, err := getVCDCluster(ctx, cs, namespace, name)
	if err != nil {
		if errors.IsNotFound(err) {
			fmt.Printf("VCDCluster not found: %s/%s, skip the delete operation\n", namespace, name)
			return nil
		}
		return fmt.Errorf("failed to get VCDCluster: %w", err)
	}

	err = cs.RESTClient().
		Delete().
		AbsPath(fmt.Sprintf("/apis/infrastructure.cluster.x-k8s.io/%s", CAPVCDObjectVersion)).
		Resource("vcdclusters").
		Namespace(namespace).
		Name(name).
		Do(ctx).Error()
	if err != nil {
		return fmt.Errorf("failed to delete VCDCluster: %w", err)
	}
	return nil
}

func getVCDCluster(ctx context.Context, cs *kubernetes.Clientset, namespace string, name string) (*infrav2.VCDCluster, error) {
	hundredKB := 100 * 1024
	vcdCluster := &infrav2.VCDCluster{}
	resp, err := cs.RESTClient().Get().
		AbsPath(fmt.Sprintf("/apis/infrastructure.cluster.x-k8s.io/%s", CAPVCDObjectVersion)).
		Resource("vcdclusters").
		Namespace(namespace).
		Name(name).
		DoRaw(ctx)

	if err != nil {
		if errors.IsNotFound(err) {
			return nil, err
		}
		return nil, fmt.Errorf("failed to get vcdcluster: %v", err)
	}
	yamlDecoder := k8syaml.NewYAMLOrJSONDecoder(bytes.NewReader(resp), hundredKB)
	if err = yamlDecoder.Decode(vcdCluster); err != nil {
		return vcdCluster, fmt.Errorf("error occurred when decoding vcdcluster: %v", err)
	}
	return vcdCluster, nil
}

func createConfigMap(ctx context.Context, cs *kubernetes.Clientset, configMap *corev1.ConfigMap, namespace string) error {
	_, err := cs.CoreV1().ConfigMaps(namespace).Create(ctx, configMap, metav1.CreateOptions{})
	if err != nil {
		return fmt.Errorf("failed to create ConfigMap: %v", err)
	}
	return nil
}

func createSecret(ctx context.Context, cs *kubernetes.Clientset, secret *corev1.Secret, namespace string) error {
	_, err := cs.CoreV1().Secrets(namespace).Create(ctx, secret, metav1.CreateOptions{})
	if err != nil {
		return fmt.Errorf("failed to create secret: %v", err)
	}
	return nil
}

func deleteSecret(ctx context.Context, cs *kubernetes.Clientset, namespace string, name string) error {
	err := cs.CoreV1().Secrets(namespace).Delete(ctx, name, metav1.DeleteOptions{})
	if err != nil {
		return fmt.Errorf("failed to delete secret: %v", err)
	}
	return nil
}

func createVCDMachineTemplate(ctx context.Context, cs *kubernetes.Clientset, vcdMachineTemplate *infrav2.VCDMachineTemplate, namespace string) error {

	body, err := json.Marshal(vcdMachineTemplate)
	if err != nil {
		return err
	}

	_, err = cs.RESTClient().
		Post().
		AbsPath(fmt.Sprintf("/apis/infrastructure.cluster.x-k8s.io/%s", CAPVCDObjectVersion)).
		Resource("vcdmachinetemplates").
		Namespace(namespace).
		Body(body).
		DoRaw(ctx)

	if err != nil {
		return fmt.Errorf("failed to create vcdmachine templace: %v", err)
	}
	return nil
}

func getVCDMachineTemplate(ctx context.Context, cs *kubernetes.Clientset, namespace string, name string) (*infrav2.VCDMachineTemplate, error) {
	hundredKB := 100 * 1024
	vcdMachineTemplate := &infrav2.VCDMachineTemplate{}
	resp, err := cs.RESTClient().
		Get().
		AbsPath(fmt.Sprintf("/apis/infrastructure.cluster.x-k8s.io/%s", CAPVCDObjectVersion)).
		Resource("vcdmachinetemplates").
		Namespace(namespace).
		Name(name).
		DoRaw(ctx)
	if err != nil {
		if errors.IsNotFound(err) {
			return nil, err
		}
		return nil, fmt.Errorf("failed to get VCDMachineTemplate: %w", err)
	}
	yamlDecoder := k8syaml.NewYAMLOrJSONDecoder(bytes.NewReader(resp), hundredKB)
	if err = yamlDecoder.Decode(vcdMachineTemplate); err != nil {
		return vcdMachineTemplate, fmt.Errorf("error occurred when decoding VCDMachineTemplate: %v", err)
	}
	return vcdMachineTemplate, nil
}

func deleteVCDMachineTemplate(ctx context.Context, cs *kubernetes.Clientset, namespace string, name string) error {
	_, err := getVCDMachineTemplate(ctx, cs, namespace, name)
	if err != nil {
		if errors.IsNotFound(err) {
			fmt.Printf("VCDMachineTemplate not found: %s/%s, skip the delete operation\n", namespace, name)
			return nil
		}
		return fmt.Errorf("failed to get VCDMachineTemplate: %v", err)
	}
	err = cs.RESTClient().
		Delete().
		AbsPath(fmt.Sprintf("/apis/infrastructure.cluster.x-k8s.io/%s", CAPVCDObjectVersion)).
		Resource("vcdmachinetemplates").
		Namespace(namespace).
		Name(name).
		Do(ctx).Error()

	if err != nil {
		return fmt.Errorf("failed to delete VCDMachineTemplate: %v", err)
	}
	return nil
}

func createKubeadmControlPlane(ctx context.Context, cs *kubernetes.Clientset, kcp *kcpv1.KubeadmControlPlane, namespace string) error {

	body, err := json.Marshal(kcp)
	if err != nil {
		return err
	}

	_, err = cs.RESTClient().
		Post().
		AbsPath(fmt.Sprintf("/apis/controlplane.cluster.x-k8s.io/%s", CAPIObjectVersion)).
		Resource("kubeadmcontrolplanes").
		Namespace(namespace).
		Body(body).
		DoRaw(ctx)

	if err != nil {
		return fmt.Errorf("failed to create kubeadmControlPlane: %v", err)
	}
	return nil
}

func getKubeadmControlPlane(ctx context.Context, cs *kubernetes.Clientset, namespace string, name string) (*kcpv1.KubeadmControlPlane, error) {
	hundredKB := 100 * 1024
	kubeadmControlPlane := &kcpv1.KubeadmControlPlane{}
	resp, err := cs.RESTClient().
		Get().
		AbsPath(fmt.Sprintf("/apis/controlplane.cluster.x-k8s.io/%s", CAPIObjectVersion)).
		Resource("kubeadmcontrolplanes").
		Namespace(namespace).
		Name(name).
		DoRaw(ctx)
	if err != nil {
		if errors.IsNotFound(err) {
			return nil, err
		}
		return nil, fmt.Errorf("failed to get KubeadmControlPlane: %w", err)
	}
	yamlDecoder := k8syaml.NewYAMLOrJSONDecoder(bytes.NewReader(resp), hundredKB)
	if err = yamlDecoder.Decode(kubeadmControlPlane); err != nil {
		return kubeadmControlPlane, fmt.Errorf("error occurred when decoding KubeadmControlPlane: %w", err)
	}
	return kubeadmControlPlane, nil
}

func deleteKubeadmControlPlane(ctx context.Context, cs *kubernetes.Clientset, namespace string, name string) error {
	_, err := getKubeadmControlPlane(ctx, cs, namespace, name)
	if err != nil {
		if errors.IsNotFound(err) {
			fmt.Printf("KubeadmControlPlane not found: %s/%s, skip the delete operation\n", namespace, name)
			return nil
		}
		return fmt.Errorf("failed to get VCDMachineTemplate: %w", err)
	}
	if _, err := getKubeadmControlPlane(ctx, cs, namespace, name); err == nil {
		_, err := cs.RESTClient().Delete().
			AbsPath(fmt.Sprintf("/apis/controlplane.cluster.x-k8s.io/%s", CAPIObjectVersion)).
			Resource("kubeadmcontrolplanes").
			Namespace(namespace).
			Name(name).
			DoRaw(ctx)
		if err != nil {
			return fmt.Errorf("failed to delete KubeadmControlPlane: %w", err)
		}
	}
	return nil
}

func createKubeadmConfigTemplate(ctx context.Context, cs *kubernetes.Clientset, kubeadmConfig *bootstrapv1.KubeadmConfigTemplate, namespace string) error {

	body, err := json.Marshal(kubeadmConfig)
	if err != nil {
		return err
	}

	_, err = cs.RESTClient().
		Post().
		AbsPath(fmt.Sprintf("/apis/bootstrap.cluster.x-k8s.io/%s", CAPIObjectVersion)).
		Resource("kubeadmconfigtemplates").
		Namespace(namespace).
		Body(body).
		DoRaw(ctx)

	if err != nil {
		return fmt.Errorf("failed to create kubeadm Config Templace: %v", err)
	}
	return nil
}

func getKubeadmConfigTemplate(ctx context.Context, cs *kubernetes.Clientset, namespace string, name string) (*bootstrapv1.KubeadmConfigTemplate, error) {
	hundredKB := 100 * 1024
	kubeadmConfigTemplate := &bootstrapv1.KubeadmConfigTemplate{}
	resp, err := cs.RESTClient().
		Get().AbsPath(fmt.Sprintf("/apis/bootstrap.cluster.x-k8s.io/%s", CAPIObjectVersion)).
		Resource("kubeadmconfigtemplates").
		Namespace(namespace).
		Name(name).
		DoRaw(ctx)
	if err != nil {
		if errors.IsNotFound(err) {
			return nil, err
		}
		return nil, fmt.Errorf("failed to retrieve KubeadmConfigTemplate: %v", err)
	}
	yamlDecoder := k8syaml.NewYAMLOrJSONDecoder(bytes.NewReader(resp), hundredKB)
	if err = yamlDecoder.Decode(kubeadmConfigTemplate); err != nil {
		return kubeadmConfigTemplate, fmt.Errorf("error occurred when decoding KubeadmConfigTemplate: %v", err)
	}
	return kubeadmConfigTemplate, nil
}

func deleteKubeadmConfigTemplate(ctx context.Context, cs *kubernetes.Clientset, namespace string, name string) error {
	_, err := getKubeadmConfigTemplate(ctx, cs, namespace, name)
	if err != nil {
		if errors.IsNotFound(err) {
			fmt.Printf("KubeadmConfigTemplate not found: %s, skip the delete operation\n", name)
			return nil
		}
		return fmt.Errorf("failed to retrieve KubeadmConfigTemplate: %v", err)
	}

	_, err = cs.RESTClient().
		Delete().
		AbsPath(fmt.Sprintf("/apis/bootstrap.cluster.x-k8s.io/%s", CAPIObjectVersion)).
		Resource("kubeadmconfigtemplates").
		Namespace(namespace).
		Name(name).
		DoRaw(ctx)
	if err != nil {
		return fmt.Errorf("failed to delete KubeadmConfigTemplate: %v", err)
	}
	return nil
}

func createMachineDeployment(ctx context.Context, cs *kubernetes.Clientset, machineDeployment *clusterv1.MachineDeployment, namespace string) error {

	body, err := json.Marshal(machineDeployment)
	if err != nil {
		return err
	}

	_, err = cs.RESTClient().
		Post().
		AbsPath(fmt.Sprintf("/apis/cluster.x-k8s.io/v1beta1/namespaces/%s/machinedeployments", namespace)).
		Body(body).
		DoRaw(ctx)

	if err != nil {
		return fmt.Errorf("failed to create machineDeployment: %v", err)
	}
	return nil
}

func getMachineDeployment(ctx context.Context, cs *kubernetes.Clientset, namespace string, name string) (*clusterv1.MachineDeployment, error) {
	hundredKB := 100 * 1024
	machineDeployment := &clusterv1.MachineDeployment{}
	resp, err := cs.RESTClient().Get().AbsPath(fmt.Sprintf("/apis/cluster.x-k8s.io/%s", CAPIObjectVersion)).
		Namespace(namespace).
		Resource("machinedeployments").
		Name(name).
		DoRaw(ctx)
	if err != nil {
		if errors.IsNotFound(err) {
			return nil, err
		}
		return nil, fmt.Errorf("failed to retrieve MachineDeployment: %v", err)
	}

	yamlDecoder := k8syaml.NewYAMLOrJSONDecoder(bytes.NewReader(resp), hundredKB)
	if err = yamlDecoder.Decode(machineDeployment); err != nil {
		return machineDeployment, fmt.Errorf("error occurred when decoding MachineDeployment: [%v]", err)
	}

	return machineDeployment, nil
}

func deleteMachineDeployment(ctx context.Context, cs *kubernetes.Clientset, namespace string, name string) error {
	_, err := getMachineDeployment(ctx, cs, namespace, name)
	if err != nil {
		if errors.IsNotFound(err) {
			fmt.Printf("MachineDeployment not found: %s, skip the delete operation\n", name)
			return nil
		}
		return fmt.Errorf("failed to retrieve MachineDeployment: %v", err)
	}

	_, err = cs.RESTClient().
		Delete().
		AbsPath(fmt.Sprintf("/apis/cluster.x-k8s.io/%s", CAPIObjectVersion)).
		Namespace(namespace).
		Resource("machinedeployments").
		Name(name).
		DoRaw(ctx)
	if err != nil {
		return fmt.Errorf("failed to delete MachineDeployment: %v", err)
	}
	return nil
}
