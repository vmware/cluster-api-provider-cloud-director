package utils

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	infrav2 "github.com/vmware/cluster-api-provider-cloud-director/api/v1beta2"
	"io"
	corev1 "k8s.io/api/core/v1"
	apierrs "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	k8stypes "k8s.io/apimachinery/pkg/types"
	k8syaml "k8s.io/apimachinery/pkg/util/yaml"
	"k8s.io/client-go/kubernetes"
	clusterv1 "sigs.k8s.io/cluster-api/api/v1beta1"
	bootstrapv1 "sigs.k8s.io/cluster-api/bootstrap/kubeadm/api/v1beta1"
	kcpv1 "sigs.k8s.io/cluster-api/controlplane/kubeadm/api/v1beta1"
)

func ApplyCAPIYaml(ctx context.Context, cs *kubernetes.Clientset, yamlContent []byte) {
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
		namespace := unstructuredObj.GetNamespace()
		switch kind {
		case Cluster:
			cluster := &clusterv1.Cluster{}
			yamlDecoder := k8syaml.NewYAMLOrJSONDecoder(bytes.NewReader(yamlBytes), hundredKB)
			if err = yamlDecoder.Decode(&cluster); err != nil {
				fmt.Println(err)
			}
			err = createCluster(ctx, cs, cluster, namespace)
		case VCDCluster:
			vcdCluster := &infrav2.VCDCluster{}
			yamlDecoder := k8syaml.NewYAMLOrJSONDecoder(bytes.NewReader(yamlBytes), hundredKB)
			if err = yamlDecoder.Decode(&vcdCluster); err != nil {
				fmt.Println(err)
			}
			err = createVCDCluster(ctx, cs, vcdCluster, namespace)
			fmt.Println(err)
		case SECRET:
			secret := &corev1.Secret{}
			yamlDecoder := k8syaml.NewYAMLOrJSONDecoder(bytes.NewReader(yamlBytes), hundredKB)
			if err = yamlDecoder.Decode(&secret); err != nil {
				fmt.Println(err)
			}
			err = createSecret(ctx, cs, secret, namespace)
			fmt.Println(err)
		case VCDMachineTemplate:
			vcdMachineTemplate := &infrav2.VCDMachineTemplate{}
			yamlDecoder := k8syaml.NewYAMLOrJSONDecoder(bytes.NewReader(yamlBytes), hundredKB)
			if err = yamlDecoder.Decode(&vcdMachineTemplate); err != nil {
				fmt.Println(err)
			}
			err = createVCDMachineTemplate(ctx, cs, vcdMachineTemplate, namespace)
			fmt.Println(err)
		case KubeadmControlPlane:
			kcp := &kcpv1.KubeadmControlPlane{}
			yamlDecoder := k8syaml.NewYAMLOrJSONDecoder(bytes.NewReader(yamlBytes), hundredKB)
			if err = yamlDecoder.Decode(&kcp); err != nil {
				fmt.Println(err)
			}
			err = createKubeadmControlPlane(ctx, cs, kcp, namespace)
			fmt.Println(err)
		case KubeadmConfigTemplate:
			kubeadmConfig := &bootstrapv1.KubeadmConfigTemplate{}
			yamlDecoder := k8syaml.NewYAMLOrJSONDecoder(bytes.NewReader(yamlBytes), hundredKB)
			if err = yamlDecoder.Decode(&kubeadmConfig); err != nil {
				fmt.Println(err)
			}
			err = createKubeadmConfigTemplate(ctx, cs, kubeadmConfig, namespace)
			fmt.Println(err)
		case MachineDeployment:
			machineDeployment := &clusterv1.MachineDeployment{}
			yamlDecoder := k8syaml.NewYAMLOrJSONDecoder(bytes.NewReader(yamlBytes), hundredKB)
			if err = yamlDecoder.Decode(&machineDeployment); err != nil {
				fmt.Println(err)
			}
			err = createMachineDeployment(ctx, cs, machineDeployment, namespace)
			fmt.Println(err)
		}

	}
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
		case SECRET:
			err = deleteSecret(ctx, cs, ns, name)
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
		AbsPath("/apis/cluster.x-k8s.io/v1beta1/").Namespace(namespace).Resource("machinedeployments").Name(resourceName).
		Body(emptyPayloadBytes).
		DoRaw(ctx)

	return err
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
func createCluster(ctx context.Context, cs *kubernetes.Clientset, cluster *clusterv1.Cluster, namespace string) error {
	body, err := json.Marshal(cluster)
	if err != nil {
		return err
	}
	_, err = cs.RESTClient().
		Post().
		AbsPath(fmt.Sprintf("/apis/cluster.x-k8s.io/v1beta1/namespaces/%s/clusters", namespace)).
		Body(body).
		DoRaw(ctx)
	return err
}

func deleteCluster(ctx context.Context, cs *kubernetes.Clientset, namespace string, name string) error {
	resp, err := cs.RESTClient().Delete().AbsPath("/apis/cluster.x-k8s.io/v1beta1").Resource("clusters").
		Namespace(namespace).Name(name).DoRaw(ctx)
	if err != nil {
		return fmt.Errorf("failed to delete cluster: %w", err)
	}
	fmt.Println(resp)

	return nil
}

func createVCDCluster(ctx context.Context, cs *kubernetes.Clientset, vcdCluster *infrav2.VCDCluster, namespace string) error {

	body, err := json.Marshal(vcdCluster)
	if err != nil {
		return err
	}

	_, err = cs.RESTClient().
		Post().
		AbsPath(fmt.Sprintf("/apis/infrastructure.cluster.x-k8s.io/v1beta2/namespaces/%s/vcdclusters", namespace)).
		Body(body).
		DoRaw(ctx)

	return err
}

func deleteVCDCluster(ctx context.Context, cs *kubernetes.Clientset, namespace string, name string) error {
	_, err := cs.RESTClient().
		Delete().AbsPath("/apis/infrastructure.cluster.x-k8s.io/v1beta2").Resource("vcdclusters").
		Namespace(namespace).Name(name).DoRaw(ctx)
	if err != nil {
		return fmt.Errorf("failed to delete VCDCluster: %v", err)
	}
	return nil
}

func getVCDCluster(ctx context.Context, cs *kubernetes.Clientset, namespace string, name string) (*infrav2.VCDCluster, error) {
	hundredKB := 100 * 1024
	vcdCluster := &infrav2.VCDCluster{}
	resp, err := cs.RESTClient().
		Delete().AbsPath("/apis/infrastructure.cluster.x-k8s.io/v1beta2").Resource("vcdclusters").
		Namespace(namespace).Name(name).DoRaw(ctx)
	yamlDecoder := k8syaml.NewYAMLOrJSONDecoder(bytes.NewReader(resp), hundredKB)
	if err = yamlDecoder.Decode(vcdCluster); err != nil {
		fmt.Println(err)
	}
	return vcdCluster, nil

}

func createSecret(ctx context.Context, cs *kubernetes.Clientset, secret *corev1.Secret, namespace string) error {
	_, err := cs.CoreV1().Secrets(namespace).Create(ctx, secret, metav1.CreateOptions{})
	return err
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
		AbsPath(fmt.Sprintf("/apis/infrastructure.cluster.x-k8s.io/v1beta2/namespaces/%s/vcdmachinetemplates", namespace)).
		Body(body).
		DoRaw(ctx)

	return err
}

func deleteVCDMachineTemplate(ctx context.Context, cs *kubernetes.Clientset, namespace string, name string) error {
	_, err := cs.RESTClient().
		Delete().AbsPath("/apis/cluster.x-k8s.io/v1beta1").Resource("vcdmachinetemplates").
		Namespace(namespace).Name(name).DoRaw(ctx)

	if err != nil {
		return fmt.Errorf("failed to delete vcdMachineTemplate: %v", err)
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
		AbsPath(fmt.Sprintf("/apis/controlplane.cluster.x-k8s.io/v1beta1/namespaces/%s/kubeadmcontrolplanes", namespace)).
		Body(body).
		DoRaw(ctx)

	if err != nil {
		return fmt.Errorf("failed to create kubeadmControlPlane: %v", err)
	}
	return nil
}

func deleteKubeadmControlPlane(ctx context.Context, cs *kubernetes.Clientset, namespace string, name string) error {
	_, err := cs.RESTClient().
		Delete().AbsPath("/apis/controlplane.cluster.x-k8s.io/v1beta1").Resource("kubeadmcontrolplanes").
		Namespace(namespace).Name(name).DoRaw(ctx)
	if err != nil {
		return fmt.Errorf("failed to delete kubeadmControlPlane: %v", err)
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
		AbsPath(fmt.Sprintf("/apis/bootstrap.cluster.x-k8s.io/v1beta1/namespaces/%s/kubeadmconfigtemplates", namespace)).
		Body(body).
		DoRaw(ctx)

	return err
}

func deleteKubeadmConfigTemplate(ctx context.Context, cs *kubernetes.Clientset, namespace string, name string) error {
	_, err := cs.RESTClient().
		Delete().AbsPath("/apis/bootstrap.cluster.x-k8s.io/v1beta1").Resource("kubeadmconfigtemplates").
		Namespace(namespace).Name(name).DoRaw(ctx)
	if err != nil {
		return fmt.Errorf("failed to delete kubeadmconfigtemplate: %v", err)
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

	return err
}

func deleteMachineDeployment(ctx context.Context, cs *kubernetes.Clientset, namespace string, name string) error {
	_, err := cs.RESTClient().
		Delete().AbsPath("/apis/cluster.x-k8s.io/v1beta1").Resource("machinedeployments").
		Namespace(namespace).Name(name).DoRaw(ctx)
	if err != nil {
		return fmt.Errorf("failed to delete MachineDeployment: %v", err)
	}
	return nil
}
