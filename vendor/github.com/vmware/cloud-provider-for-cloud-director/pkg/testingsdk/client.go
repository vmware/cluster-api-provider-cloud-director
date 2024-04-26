package testingsdk

import (
	"context"
	"fmt"
	"github.com/vmware/cloud-provider-for-cloud-director/pkg/vcdsdk"
	"gopkg.in/yaml.v3"
	appsv1 "k8s.io/api/apps/v1"
	apiv1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/apimachinery/pkg/version"
	"k8s.io/client-go/kubernetes"
	restclient "k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
)

type TestClient struct {
	VcdClient   *vcdsdk.Client
	Cs          kubernetes.Interface
	ClusterId   string
	ClusterName string
	Config      *restclient.Config
}

type VCDAuthParams struct {
	Host           string
	OvdcIdentifier string
	OrgName        string
	Username       string
	RefreshToken   string
	UserOrg        string
	GetVdcClient   bool // This will need to be set to true as it's needed for CSI, but may not be needed for other use cases
}

type DeployParams struct {
	Name            string
	Labels          map[string]string
	VolumeParams    VolumeParams
	ContainerParams ContainerParams
}
type VolumeParams struct {
	VolumeName string
	PvcRef     string
	MountPath  string
}

type ContainerParams struct {
	ContainerName  string
	ContainerImage string
	ContainerPort  int32
	Args           []string
}

func NewTestClient(params *VCDAuthParams, clusterId string) (*TestClient, error) {
	client, err := getTestVCDClient(params)
	if err != nil {
		return nil, fmt.Errorf("error occured while generating client using [%s:%s] for cluster [%s]: [%v]", params.Username, params.UserOrg, clusterId, err)
	}

	kubeConfig, err := GetKubeconfigFromRDEId(context.TODO(), client, clusterId)
	if err != nil {
		return nil, fmt.Errorf("unable to get kubeconfig from RDE [%s]: [%v]", clusterId, err)
	}
	config, err := clientcmd.RESTConfigFromKubeConfig([]byte(kubeConfig))
	if err != nil {
		return nil, fmt.Errorf("unable to create RESTConfig using kubeconfig from RDE: [%v]", err)
	}
	cs, err := kubernetes.NewForConfig(config)
	if err != nil {
		return nil, fmt.Errorf("unable to create clientset using RESTConfig generated from kubeconfig for cluster [%s]: [%v]", clusterId, err)
	}

	clusterName, err := getClusterNameById(context.TODO(), client, clusterId)
	if err != nil {
		return nil, fmt.Errorf("unable to get Cluster Name by Cluster Id [%s]: [%v]", clusterId, err)
	}
	return &TestClient{
		VcdClient:   client,
		Cs:          cs,
		ClusterId:   clusterId,
		ClusterName: clusterName,
		Config:      config,
	}, nil
}

func createKubeClient(kubeConfig string) (kubernetes.Interface, error) {
	config, err := clientcmd.RESTConfigFromKubeConfig([]byte(kubeConfig))
	if err != nil {
		return nil, fmt.Errorf("unable to create RESTConfig using kubeconfig from RDE: [%v]", err)
	}
	return kubernetes.NewForConfig(config)
}

func (tc *TestClient) CreateNameSpace(ctx context.Context, nsName string) (*apiv1.Namespace, error) {
	ns, err := createNameSpace(ctx, nsName, tc.Cs.(*kubernetes.Clientset))
	if err != nil {
		return nil, fmt.Errorf("error creating NameSpace [%s] for cluster [%s(%s)]: [%v]", nsName, tc.ClusterName, tc.ClusterId, err)
	}
	return ns, nil
}

func (tc *TestClient) CreateDeployment(ctx context.Context, params *DeployParams, nameSpace string) (*appsv1.Deployment, error) {
	deployment, err := createDeployment(ctx, tc.Cs.(*kubernetes.Clientset), params, nameSpace)
	if err != nil {
		return nil, fmt.Errorf("error creating Deployment [%s] for cluster [%s(%s)]: [%v]", params.Name, tc.ClusterName, tc.ClusterId, err)
	}
	return deployment, nil
}

func (tc *TestClient) CreateLoadBalancerService(ctx context.Context, nameSpace string, serviceName string, annotations map[string]string, labels map[string]string, servicePort []apiv1.ServicePort, loadBalancerIP string) (*apiv1.Service, error) {
	lbService, err := createLoadBalancerService(ctx, tc.Cs.(*kubernetes.Clientset), nameSpace, serviceName, annotations, labels, servicePort, loadBalancerIP)
	if err != nil {
		return nil, fmt.Errorf("error creating LoadBalancer Service [%s] for cluster [%s(%s)]: [%v]", serviceName, tc.ClusterName, tc.ClusterId, err)
	}
	return lbService, nil
}

func (tc *TestClient) DeleteDeployment(ctx context.Context, nameSpace string, deploymentName string) error {
	err := deleteDeployment(ctx, tc.Cs.(*kubernetes.Clientset), nameSpace, deploymentName)
	if err != nil {
		return fmt.Errorf("error deleting Deployment [%s] for cluster [%s(%s)]: [%v]", deploymentName, tc.ClusterName, tc.ClusterId, err)
	}
	return nil

}

func (tc *TestClient) DeleteNameSpace(ctx context.Context, nameSpace string) error {
	err := deleteNameSpace(ctx, tc.Cs.(*kubernetes.Clientset), nameSpace)
	if err != nil {
		return fmt.Errorf("error deleting NameSpace [%s] for cluster [%s(%s)]: [%v]", nameSpace, tc.ClusterName, tc.ClusterId, err)
	}
	return nil
}

func (tc *TestClient) DeleteService(ctx context.Context, nameSpace string, serviceName string) error {
	err := deleteService(ctx, tc.Cs.(*kubernetes.Clientset), nameSpace, serviceName)
	if err != nil {
		return fmt.Errorf("error deleting Service [%s] for cluster [%s(%s)]: [%v]", serviceName, tc.ClusterName, tc.ClusterId, err)
	}
	return nil
}

func (tc *TestClient) GetWorkerNodes(ctx context.Context) ([]apiv1.Node, error) {
	wnPool, err := getWorkerNodes(ctx, tc.Cs.(*kubernetes.Clientset))
	if err != nil {
		if err == ResourceNotFound {
			return nil, err
		}
		return nil, fmt.Errorf("error getting Worker Node Pool for cluster [%s(%s)]: [%v]", tc.ClusterName, tc.ClusterId, err)
	}
	return wnPool, nil
}

func (tc *TestClient) GetNodes(ctx context.Context) ([]apiv1.Node, error) {
	nPool, err := getNodes(ctx, tc.Cs.(*kubernetes.Clientset))
	if err != nil {
		if err == ResourceNotFound {
			return nil, err
		}
		return nil, fmt.Errorf("error getting Node Pool for cluster [%s(%s)]: [%v]", tc.ClusterName, tc.ClusterId, err)
	}
	return nPool, nil
}

func (tc *TestClient) GetDeployment(ctx context.Context, nameSpace string, deployName string) (*appsv1.Deployment, error) {
	deployment, err := getDeployment(ctx, tc.Cs.(*kubernetes.Clientset), nameSpace, deployName)
	if err != nil {
		if err == ResourceNotFound {
			return nil, err
		}
		return nil, fmt.Errorf("error getting Deployment [%s] for cluster [%s(%s)]: [%v]", deployName, tc.ClusterName, tc.ClusterId, err)
	}
	return deployment, nil
}

func (tc *TestClient) GetService(ctx context.Context, nameSpace string, serviceName string) (*apiv1.Service, error) {
	svc, err := getService(ctx, tc.Cs.(*kubernetes.Clientset), nameSpace, serviceName)
	if err != nil {
		if err == ResourceNotFound {
			return nil, err
		}
		return nil, fmt.Errorf("error getting Service [%s] for cluster [%s(%s)]: [%v]", serviceName, tc.ClusterName, tc.ClusterId, err)
	}
	return svc, nil
}

func (tc *TestClient) GetConfigMap(namespace, name string) (*apiv1.ConfigMap, error) {
	return tc.Cs.CoreV1().ConfigMaps(namespace).Get(context.TODO(), name, metav1.GetOptions{})
}

func (tc *TestClient) GetK8sVersion() (*version.Info, error) {
	return tc.Cs.(*kubernetes.Clientset).Discovery().ServerVersion()
}

// GetZoneMapFromZoneConfigMap reads the zone config map and returns a map of zoneName -> ovdcName
func (tc *TestClient) GetZoneMapFromZoneConfigMap(zoneCM *apiv1.ConfigMap) (map[string]string, error) {
	data := zoneCM.Data

	zoneYaml, ok := data["vcloud-cse-zones.yaml"]
	if !ok {
		return nil, fmt.Errorf("no data present")
	}
	var zoneCfgMap map[string]interface{}
	err := yaml.Unmarshal([]byte(zoneYaml), &zoneCfgMap)
	if err != nil {
		return nil, fmt.Errorf("err occurred: [%v]", err)
	}
	zoneInfoListIf, ok := zoneCfgMap["zones"]
	if !ok {
		return nil, fmt.Errorf("zone config map doesn't have zone list")
	}

	zoneInfoList, ok := zoneInfoListIf.([]interface{})
	if !ok {
		return nil, fmt.Errorf("failed to convert zoneInfoList interface to array")
	}

	zoneMap := make(map[string]string)
	for _, zoneInfoIf := range zoneInfoList {
		zoneInfo, ok := zoneInfoIf.(map[string]interface{})
		if !ok {
			return nil, fmt.Errorf("failed to convert zone entry in zone config map [%v] to map[string]string", zoneInfoIf)
		}
		name, ok := zoneInfo["name"]
		if !ok {
			return nil, fmt.Errorf("zone entry in zone map doesn't have a zone name")
		}
		ovdcName, ok := zoneInfo["ovdcName"]
		if !ok {
			return nil, fmt.Errorf("zone entry in zone map doesn't have OVDC name")
		}
		nameStr := name.(string)
		ovdcNameStr := ovdcName.(string)
		zoneMap[nameStr] = ovdcNameStr
	}
	return zoneMap, nil
}

func (tc *TestClient) GetIpamSubnetFromConfigMap(cm *apiv1.ConfigMap) (string, error) {
	data := cm.Data
	ccmYaml, ok := data["vcloud-ccm-config.yaml"]
	if !ok {
		return "", fmt.Errorf("no data present")
	}

	var ccmCfgMap map[string]interface{}
	err := yaml.Unmarshal([]byte(ccmYaml), &ccmCfgMap)
	if err != nil {
		return "", fmt.Errorf("err occurred: [%v]", err)
	}

	for key, val := range ccmCfgMap {
		if key == "loadbalancer" {
			lbDataMap, ok := val.(map[string]interface{})
			if !ok {
				return "", fmt.Errorf("unable to convert loadbalancer content to data map")
			}
			for k, v := range lbDataMap {
				if k == "vipSubnet" {
					return v.(string), nil
				}
			}
		}
	}
	return "", fmt.Errorf("unable to find vipSubnet from ConfigMap [%s]", cm.Name)
}

func (tc *TestClient) GetNetworkNameFromConfigMap(cm *apiv1.ConfigMap) (string, error) {
	data := cm.Data
	ccmYaml, ok := data["vcloud-ccm-config.yaml"]
	if !ok {
		return "", fmt.Errorf("no data present")
	}

	var result map[string]interface{}
	err := yaml.Unmarshal([]byte(ccmYaml), &result)
	if err != nil {
		return "", fmt.Errorf("err occurred: [%v]", err)
	}

	for key, val := range result {
		if key == "loadbalancer" {
			lbDataMap, ok := val.(map[string]interface{})
			if !ok {
				return "", fmt.Errorf("unable to convert loadbalancer content to data map")
			}
			for k, v := range lbDataMap {
				if k == "network" {
					return v.(string), nil
				}
			}
		}
	}
	return "", nil
}

func (tc *TestClient) WaitForDeploymentReady(ctx context.Context, nameSpace string, deployName string) error {
	err := waitForDeploymentReady(ctx, tc.Cs.(*kubernetes.Clientset), nameSpace, deployName)
	if err != nil {
		return fmt.Errorf("error querying Deployment [%s] status for cluster [%s(%s)]: [%v]", deployName, tc.ClusterName, tc.ClusterId, err)
	}
	return nil
}

func (tc *TestClient) WaitForWorkerNodeReady(ctx context.Context, workerNode *apiv1.Node) error {
	err := wait.PollImmediate(defaultNodeInterval, defaultNodeReadyTimeout, func() (bool, error) {
		nodes, err := tc.GetWorkerNodes(ctx)
		if err != nil {
			return false, fmt.Errorf("error getting a list of nodes from cluster [%s(%s)]: [%v]", tc.ClusterName, tc.ClusterId, err)
		}

		for _, node := range nodes {
			if node.Name == workerNode.Name {
				for _, condition := range node.Status.Conditions {
					if condition.Type == apiv1.NodeReady && condition.Status == apiv1.ConditionTrue {
						return true, nil
					}
				}
			}
		}
		return false, nil
	})
	if err != nil {
		return fmt.Errorf("error querying node [%s] status for cluster [%s(%s)]: [%v]", workerNode.Name, tc.ClusterName, tc.ClusterId, err)
	}
	return nil
}

func (tc *TestClient) WaitForWorkerNodePhaseRunning(ctx context.Context, workerNode *apiv1.Node) error {
	err := wait.PollImmediate(defaultNodeInterval, defaultNodeReadyTimeout, func() (bool, error) {
		nodes, err := tc.GetWorkerNodes(ctx)
		if err != nil {
			return false, fmt.Errorf("error getting a list of nodes from cluster [%s(%s)]: [%v]", tc.ClusterName, tc.ClusterId, err)
		}

		for _, node := range nodes {
			if node.Name == workerNode.Name && node.Status.Phase == apiv1.NodeRunning {
				return true, nil
			}
		}
		return false, nil
	})
	if err != nil {
		return fmt.Errorf("error querying node [%s] status for cluster [%s(%s)]: [%v]", workerNode.Name, tc.ClusterName, tc.ClusterId, err)
	}
	return nil
}

// WaitForWorkerNodeNotReady we cannot use negate result from WaitForWorkerNodeReady() Set different RetryTimeInterval and avoid timeout error
func (tc *TestClient) WaitForWorkerNodeNotReady(ctx context.Context, workerNode *apiv1.Node) error {
	err := wait.PollImmediate(defaultNodeInterval, defaultNodeNotReadyTimeout, func() (bool, error) {
		nodes, err := tc.GetWorkerNodes(ctx)
		if err != nil {
			return false, fmt.Errorf("error getting a list of nodes from cluster [%s(%s)]: [%v]", tc.ClusterName, tc.ClusterId, err)
		}

		for _, node := range nodes {
			if node.Name == workerNode.Name {
				for _, condition := range node.Status.Conditions {
					if condition.Type == apiv1.NodeReady && condition.Status != apiv1.ConditionTrue {
						return true, nil
					}
				}
			}
		}
		return false, nil
	})
	if err != nil {
		return fmt.Errorf("error querying node [%s] status for cluster [%s(%s)]: [%v]", workerNode.Name, tc.ClusterName, tc.ClusterId, err)
	}
	return nil
}

func (tc *TestClient) WaitForExtIP(namespace string, name string) (string, error) {
	svc, err := waitForServiceExposure(tc.Cs, namespace, name)
	if err != nil {
		return "", err
	}

	if svc == nil {
		return "", fmt.Errorf("the service is nil")
	}
	// We can safely return below as we handled the len(IngressList) check in waitServiceExposure()
	return svc.Status.LoadBalancer.Ingress[0].IP, nil
}
