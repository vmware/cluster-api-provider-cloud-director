package testingsdk

import (
	"context"
	"errors"
	"fmt"
	appsv1 "k8s.io/api/apps/v1"
	apiv1 "k8s.io/api/core/v1"
	apierrs "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	utilnet "k8s.io/apimachinery/pkg/util/net"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/client-go/kubernetes"
	"k8s.io/klog"
	"time"
)

var (
	ResourceExisted             = errors.New("[REX] resource is already existed")
	ResourceNotFound            = errors.New("[RNF] resource is not found")
	ResourceNameNull            = errors.New("[RNN] resource name is null")
	ControlPlaneLabel           = "node-role.kubernetes.io/control-plane"
	defaultRetryInterval        = 10 * time.Second
	defaultRetryTimeout         = 600 * time.Second
	defaultLongRetryInterval    = 20 * time.Second
	defaultLongRetryTimeout     = 900 * time.Second
	defaultNodeInterval         = 2 * time.Second
	defaultNodeReadyTimeout     = 20 * time.Minute
	defaultNodeNotReadyTimeout  = 8 * time.Minute
	defaultServiceRetryInterval = 10 * time.Second
	defaultServiceRetryTimeout  = 10 * time.Minute
)

func waitForServiceExposure(cs kubernetes.Interface, namespace string, name string) (*apiv1.Service, error) {
	var svc *apiv1.Service
	var err error

	err = wait.PollImmediate(defaultServiceRetryInterval, defaultServiceRetryTimeout, func() (bool, error) {
		svc, err = cs.CoreV1().Services(namespace).Get(context.TODO(), name, metav1.GetOptions{})
		if err != nil {
			// If our error is retryable, it's not a major error. So we do not need to return it as an error.
			if IsRetryableError(err) {
				return false, nil
			}
			return false, err
		}

		IngressList := svc.Status.LoadBalancer.Ingress
		if len(IngressList) == 0 {
			// we'll store an error here and continue to retry until timeout, if this was where we time out eventually, we will return an error at the end
			err = fmt.Errorf("cannot find Ingress components after duration: [%d] minutes", defaultServiceRetryTimeout/time.Minute)
			return false, nil
		}

		ip := svc.Status.LoadBalancer.Ingress[0].IP
		return ip != "", nil // Once we have our IP, we can terminate the condition for polling and return the service
	})

	if err != nil {
		return nil, err
	}
	return svc, nil
}

func waitForDeploymentReady(ctx context.Context, k8sClient *kubernetes.Clientset, nameSpace string, deployName string) error {
	err := wait.PollImmediate(defaultRetryInterval, defaultRetryTimeout, func() (bool, error) {
		options := metav1.ListOptions{
			LabelSelector: fmt.Sprintf("app=%s", deployName),
		}
		podList, err := k8sClient.CoreV1().Pods(nameSpace).List(ctx, options)
		if err != nil {
			if IsRetryableError(err) {
				return false, nil
			}
			return false, fmt.Errorf("unexpected error occurred while getting deployment [%s]", deployName)
		}
		podCount := len(podList.Items)
		containerCount := 0
		podsReady := 0
		containersReady := 0
		for _, pod := range (*podList).Items {
			// Add the total amount of containers per pod
			containerCount += len(pod.Spec.Containers)
			containersReadyFromCurrentPod := 0
			if pod.Status.Phase == apiv1.PodRunning {
				// Ref: https://kubernetes.io/docs/concepts/workloads/pods/pod-lifecycle/
				// Pod running state can mean: at least one container is still running, or is in the process of starting or restarting.
				// It's possible that the container is just starting up and not fully ready, so we should also check if ContainerStatus is ready.
				for _, container := range pod.Status.ContainerStatuses {
					if container.Ready {
						containersReadyFromCurrentPod++
					}
				}

				if containersReadyFromCurrentPod == len(pod.Spec.Containers) {
					podsReady++
					containersReady += containersReadyFromCurrentPod
				}
			}
		}
		// It is possible to have a race condition where Pods are not up yet and in testing code it can think that the Deployment is ready
		// Because there are no Pods up yet, though Deployment has been applied.
		if podCount == 0 {
			fmt.Printf("no containers or pods are ready yet")
			return false, nil
		}

		if podsReady < podCount || containersReady < containerCount {
			fmt.Printf("running pods: %v < %v; ready containers: %v < %v\n", podsReady, podCount, containersReady, containerCount)
			return false, nil
		}
		return true, nil
	})
	return err
}

func waitForDeploymentDeleted(ctx context.Context, k8sClient *kubernetes.Clientset, nameSpace string, deployName string) (bool, error) {
	err := wait.PollImmediate(defaultRetryInterval, defaultRetryTimeout, func() (bool, error) {
		_, err := getDeployment(ctx, k8sClient, nameSpace, deployName)
		if err != nil {
			if err == ResourceNotFound {
				return true, nil
			}
			if IsRetryableError(err) {
				return false, nil
			}
			return false, fmt.Errorf("unexpected error occurred while getting deployment [%s]", deployName)
		}
		return false, nil
	})
	if err != nil {
		return false, fmt.Errorf("error occurred while checking deployment status: %v", err)
	}
	return true, nil
}

func waitForServiceDeleted(ctx context.Context, k8sClient *kubernetes.Clientset, nameSpace string, serviceName string) (bool, error) {
	err := wait.PollImmediate(defaultRetryInterval, defaultRetryTimeout, func() (bool, error) {
		_, err := getService(ctx, k8sClient, nameSpace, serviceName)
		if err != nil {
			if err == ResourceNotFound {
				return true, nil
			}
			if IsRetryableError(err) {
				return false, nil
			}
			return false, fmt.Errorf("unexpected error occurred while getting service [%s]", serviceName)
		}
		return false, nil
	})
	if err != nil {
		return false, fmt.Errorf("error occurred while checking serviceName status: %v", err)
	}
	return true, nil
}

func waitForNameSpaceDeleted(ctx context.Context, k8sClient *kubernetes.Clientset, nameSpace string) (bool, error) {
	err := wait.PollImmediate(defaultLongRetryInterval, defaultLongRetryTimeout, func() (bool, error) {
		_, err := k8sClient.CoreV1().Namespaces().Get(ctx, nameSpace, metav1.GetOptions{})
		if err != nil {
			if apierrs.IsNotFound(err) {
				return true, nil
			}
			if IsRetryableError(err) {
				return false, nil
			}
			return false, fmt.Errorf("unexpected error occurred while getting namespace [%s]", nameSpace)
		}
		return false, nil
	})
	if err != nil {
		return false, fmt.Errorf("error occurred while checking namespace status: %v", err)
	}
	return true, nil
}

func IsRetryableError(err error) bool {
	if apierrs.IsInternalError(err) || apierrs.IsTimeout(err) || apierrs.IsServerTimeout(err) ||
		apierrs.IsTooManyRequests(err) || utilnet.IsProbableEOF(err) || utilnet.IsConnectionReset(err) {
		return true
	}
	return false
}

func getDeployment(ctx context.Context, k8sClient *kubernetes.Clientset, nameSpace string, deployName string) (*appsv1.Deployment, error) {
	if deployName == "" {
		return nil, ResourceNameNull
	}
	deployment, err := k8sClient.AppsV1().Deployments(nameSpace).Get(ctx, deployName, metav1.GetOptions{})
	if err != nil {
		if apierrs.IsNotFound(err) {
			return nil, ResourceNotFound
		}
		return nil, err
	}
	return deployment, nil
}

func getService(ctx context.Context, k8sClient *kubernetes.Clientset, nameSpace string, serviceName string) (*apiv1.Service, error) {
	if serviceName == "" {
		return nil, ResourceNameNull
	}
	svc, err := k8sClient.CoreV1().Services(nameSpace).Get(ctx, serviceName, metav1.GetOptions{})
	if err != nil {
		if apierrs.IsNotFound(err) {
			return nil, ResourceNotFound
		}
		return nil, err
	}
	return svc, nil
}

func getWorkerNodes(ctx context.Context, k8sClient *kubernetes.Clientset) ([]apiv1.Node, error) {
	var workerNodes []apiv1.Node
	var timeoutSeconds int64 = 600
	nodes, err := k8sClient.CoreV1().Nodes().List(ctx, metav1.ListOptions{TimeoutSeconds: &timeoutSeconds})
	if err != nil {
		return workerNodes, fmt.Errorf("error occurred while getting nodes: [%v]", err)
	}
	for _, node := range nodes.Items {
		_, ok := node.Labels[ControlPlaneLabel]
		if !ok {
			workerNodes = append(workerNodes, node)
		}
	}
	return workerNodes, nil
}

func getNodes(ctx context.Context, k8sClient *kubernetes.Clientset) ([]apiv1.Node, error) {
	var allNodes []apiv1.Node
	var timeoutSeconds int64 = 600
	nodes, err := k8sClient.CoreV1().Nodes().List(ctx, metav1.ListOptions{TimeoutSeconds: &timeoutSeconds})
	if err != nil {
		return allNodes, fmt.Errorf("error occurred while getting nodes: [%v]", err)
	}
	for _, node := range nodes.Items {
		allNodes = append(allNodes, node)
	}
	return allNodes, nil
}

func createNameSpace(ctx context.Context, nsName string, k8sClient *kubernetes.Clientset) (*apiv1.Namespace, error) {
	if nsName == "" {
		return nil, ResourceNameNull
	}
	namespace := &apiv1.Namespace{
		ObjectMeta: metav1.ObjectMeta{
			Name: nsName,
		},
	}
	ns, err := k8sClient.CoreV1().Namespaces().Create(ctx, namespace, metav1.CreateOptions{})
	if err != nil {
		return nil, fmt.Errorf("error occurred while creating namespace [%s]: [%v]", nsName, err)
	}
	return ns, nil
}

func createDeployment(ctx context.Context, k8sClient *kubernetes.Clientset, params *DeployParams, nameSpace string) (*appsv1.Deployment, error) {
	if params.Name == "" {
		return nil, ResourceNameNull
	}
	if nameSpace == "" {
		nameSpace = apiv1.NamespaceDefault
	}
	deployment := &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:      params.Name,
			Namespace: nameSpace,
		},
		Spec: appsv1.DeploymentSpec{
			Selector: &metav1.LabelSelector{
				MatchLabels: params.Labels,
			},
			Strategy: appsv1.DeploymentStrategy{
				Type: appsv1.RecreateDeploymentStrategyType,
			},
			Template: apiv1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{
					Labels: params.Labels,
				},
				Spec: apiv1.PodSpec{
					Containers: []apiv1.Container{
						{
							Image:           params.ContainerParams.ContainerImage,
							ImagePullPolicy: apiv1.PullAlways,
							Name:            params.ContainerParams.ContainerName,
							Args:            params.ContainerParams.Args,
							Ports: []apiv1.ContainerPort{
								{
									ContainerPort: params.ContainerParams.ContainerPort,
								},
							},
						},
					},
				},
			},
		},
	}

	if params.VolumeParams.VolumeName != "" && params.VolumeParams.PvcRef != "" && params.VolumeParams.MountPath != "" {
		deployment.Spec.Template.Spec.Containers[0].VolumeMounts = []apiv1.VolumeMount{
			{
				Name:      params.VolumeParams.VolumeName,
				MountPath: params.VolumeParams.MountPath,
			},
		}
		deployment.Spec.Template.Spec.Volumes = []apiv1.Volume{
			{
				Name: params.VolumeParams.VolumeName,
				VolumeSource: apiv1.VolumeSource{
					PersistentVolumeClaim: &apiv1.PersistentVolumeClaimVolumeSource{
						ClaimName: params.VolumeParams.PvcRef,
					},
				},
			},
		}
	}

	newDeployment, err := k8sClient.AppsV1().Deployments(nameSpace).Create(ctx, deployment, metav1.CreateOptions{})
	if err != nil {
		return nil, fmt.Errorf("error occurred while creating deployment [%s]: [%v]", params.Name, err)
	}
	return newDeployment, nil
}

func createLoadBalancerService(ctx context.Context, k8sClient *kubernetes.Clientset, nameSpace string, serviceName string, annotations map[string]string, labels map[string]string, servicePort []apiv1.ServicePort, loadBalancerIP string) (*apiv1.Service, error) {
	if serviceName == "" {
		return nil, ResourceNameNull
	}
	if nameSpace == "" {
		nameSpace = apiv1.NamespaceDefault
	}
	svc := &apiv1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:        serviceName,
			Namespace:   nameSpace,
			Annotations: annotations,
			Labels:      labels,
		},
		Spec: apiv1.ServiceSpec{
			Ports:          servicePort,
			Selector:       labels,
			Type:           "LoadBalancer",
			LoadBalancerIP: loadBalancerIP,
		},
	}
	newSVC, err := k8sClient.CoreV1().Services(nameSpace).Create(ctx, svc, metav1.CreateOptions{})
	if err != nil {
		return nil, fmt.Errorf("error occurred while creating service [%s]: [%v]", serviceName, err)
	}
	return newSVC, nil
}

func deleteDeployment(ctx context.Context, k8sClient *kubernetes.Clientset, nameSpace string, deploymentName string) error {
	_, err := getDeployment(ctx, k8sClient, nameSpace, deploymentName)
	if err != nil {
		if err == ResourceNotFound {
			return fmt.Errorf("the deployment [%s] does not exist", deploymentName)
		}
		klog.Infof("error occurred while getting deployment [%s]: [%v]", deploymentName, err)
	}
	err = k8sClient.AppsV1().Deployments(nameSpace).Delete(ctx, deploymentName, metav1.DeleteOptions{})
	if err != nil {
		return fmt.Errorf("failed to delete deployment [%s]", deploymentName)
	}
	deploymentDeleted, err := waitForDeploymentDeleted(ctx, k8sClient, nameSpace, deploymentName)
	if err != nil {
		return fmt.Errorf("error occurred while deleting deployment [%s]: [%v]", deploymentName, err)
	}
	if !deploymentDeleted {
		return fmt.Errorf("deployment [%s] still exists", deploymentName)
	}
	return nil
}

func deleteNameSpace(ctx context.Context, k8sClient *kubernetes.Clientset, nameSpace string) error {
	err := k8sClient.CoreV1().Namespaces().Delete(ctx, nameSpace, metav1.DeleteOptions{})
	if err != nil {
		return fmt.Errorf("failed to delete namespace [%s]", nameSpace)
	}
	namespaceDeleted, err := waitForNameSpaceDeleted(ctx, k8sClient, nameSpace)
	if err != nil {
		return fmt.Errorf("error occurred while deleting namespace [%s]: [%v]", nameSpace, err)
	}
	if !namespaceDeleted {
		return fmt.Errorf("namespace [%s] still exists", nameSpace)
	}
	return nil
}

func deleteService(ctx context.Context, k8sClient *kubernetes.Clientset, nameSpace string, serviceName string) error {
	_, err := getService(ctx, k8sClient, nameSpace, serviceName)
	if err != nil {
		if err == ResourceNotFound {
			return fmt.Errorf("the service [%s] does not exist", serviceName)
		}
		klog.Infof("error occurred while getting service [%s]: [%v]", serviceName, err)
	}
	err = k8sClient.CoreV1().Services(nameSpace).Delete(ctx, serviceName, metav1.DeleteOptions{})
	if err != nil {
		return fmt.Errorf("failed to delete service [%s]", serviceName)
	}
	serviceDeleted, err := waitForServiceDeleted(ctx, k8sClient, nameSpace, serviceName)
	if err != nil {
		return fmt.Errorf("error occurred while deleting service [%s]: [%v]", serviceName, err)
	}
	if !serviceDeleted {
		return fmt.Errorf("service [%s] still exists", serviceName)
	}
	return nil
}
