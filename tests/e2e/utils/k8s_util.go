package utils

import (
	"bufio"
	"bytes"
	"context"
	_ "embed" // this needs go 1.16+
	"fmt"
	infrav2 "github.com/vmware/cluster-api-provider-cloud-director/api/v1beta2"
	"io"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	apierrs "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	k8syaml "k8s.io/apimachinery/pkg/util/yaml"
	"k8s.io/client-go/kubernetes"
	runtimeclient "sigs.k8s.io/controller-runtime/pkg/client"
	"text/template"
)

const (
	CAPIObjectVersion   = "v1beta1"
	CAPVCDObjectVersion = "v1beta2"
	FieldManager        = "rdeProjector"
)

//go:embed config_map_files/vcloud-ccm-config.yaml
var ccmConfigMapYamlTemplate string

//go:embed config_map_files/vcloud-csi-config.yaml
var csiConfigMapYamlTemplate string

// ApplyCAPIYaml applies the Kubernetes YAML content to create objects using the provided runtime client.
// It reads the YAML content, decodes each object, and creates them in the cluster using the runtime client.
// If an error occurs during decoding or creation, it returns the error.
func ApplyCAPIYaml(ctx context.Context, r runtimeclient.Client, yamlContent []byte) error {

	yamlReader := k8syaml.NewYAMLReader(bufio.NewReader(bytes.NewReader(yamlContent)))
	hundredKB := 100 * 1024

	for {
		yamlBytes, err := yamlReader.Read()
		if err == io.EOF {
			break
		} else if err != nil {
			return err
		}

		yamlDecoder := k8syaml.NewYAMLOrJSONDecoder(bytes.NewReader(yamlBytes), hundredKB)
		unstructuredObj := unstructured.Unstructured{}
		if err = yamlDecoder.Decode(&unstructuredObj); err != nil {
			fmt.Println(err)
			continue
		}
		err = r.Create(ctx, &unstructuredObj, &runtimeclient.CreateOptions{
			FieldManager: FieldManager,
		})
		if err != nil {
			return err
		}
	}

	return nil
}

func DeleteCAPIYaml(ctx context.Context, r runtimeclient.Client, yamlContent []byte) error {
	yamlReader := k8syaml.NewYAMLReader(bufio.NewReader(bytes.NewReader(yamlContent)))

	for {
		yamlBytes, err := yamlReader.Read()
		if err == io.EOF {
			break
		} else if err != nil {
			return fmt.Errorf("error reading YAML: %w", err)
		}

		yamlDecoder := k8syaml.NewYAMLOrJSONDecoder(bytes.NewReader(yamlBytes), 0)
		unstructuredObj := unstructured.Unstructured{}
		if err = yamlDecoder.Decode(&unstructuredObj); err != nil {
			fmt.Println(err)
			continue
		}

		// Check if the object is a secret
		if unstructuredObj.GetKind() == SECRET {
			fmt.Printf("Skipping deletion of secret: %s/%s\n", unstructuredObj.GetNamespace(), unstructuredObj.GetName())
			continue
		}

		objKey := runtimeclient.ObjectKey{
			Namespace: unstructuredObj.GetNamespace(),
			Name:      unstructuredObj.GetName(),
		}
		if err := r.Get(ctx, objKey, &unstructuredObj); err != nil {
			if !errors.IsNotFound(err) {
				fmt.Printf("Failed to get object: %s/%s\n", unstructuredObj.GetNamespace(), unstructuredObj.GetName())
				return fmt.Errorf("failed to get object: %s/%s: %w", unstructuredObj.GetNamespace(), unstructuredObj.GetName(), err)
			}
			continue
		}

		fmt.Printf("Deleting object: %s/%s\n", unstructuredObj.GetNamespace(), unstructuredObj.GetName())
		err = r.Delete(ctx, &unstructuredObj, &runtimeclient.DeleteOptions{})
		if err != nil {
			fmt.Printf("Failed to delete object: %s/%s\n", unstructuredObj.GetNamespace(), unstructuredObj.GetName())
			return fmt.Errorf("failed to delete object: %s/%s: %w", unstructuredObj.GetNamespace(), unstructuredObj.GetName(), err)
		}
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

func getVCDCluster(ctx context.Context, r runtimeclient.Client, namespace, name string) (*infrav2.VCDCluster, error) {
	vcdCluster := &infrav2.VCDCluster{}
	err := r.Get(ctx, runtimeclient.ObjectKey{
		Namespace: namespace,
		Name:      name,
	}, vcdCluster)
	if err != nil {
		fmt.Printf("Failed to get object: %s/%s\n", namespace, name)
		return vcdCluster, fmt.Errorf("failed to get object: %s/%s: %v", namespace, name, err)
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
