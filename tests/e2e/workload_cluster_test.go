package e2e

import (
	"context"
	"fmt"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	"github.com/vmware/cloud-provider-for-cloud-director/pkg/testingsdk"
	infrav2 "github.com/vmware/cluster-api-provider-cloud-director/api/v1beta2"
	"github.com/vmware/cluster-api-provider-cloud-director/tests/e2e/utils"
	"k8s.io/apimachinery/pkg/runtime"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/rest"
	"os"
	clusterv1beta1 "sigs.k8s.io/cluster-api/api/v1beta1"
	kcfg "sigs.k8s.io/cluster-api/util/kubeconfig"
	runtimeclient "sigs.k8s.io/controller-runtime/pkg/client"
)

var _ = Describe("Workload Cluster Life cycle management", func() {
	When("create, resize, and delete the workload cluster", func() {
		var (
			runtimeClient          runtimeclient.Client
			testClient             *testingsdk.TestClient
			ctx                    context.Context
			kubeConfig             []byte
			capiYaml               []byte
			cs                     *kubernetes.Clientset
			workloadClientSet      *kubernetes.Clientset
			desiredWorkerNodeCount int64
			clusterName            string
			clusterNameSpace       string
			vcdCluster             *infrav2.VCDCluster
			rdeID                  string
		)

		BeforeEach(func() {
			var err error
			var restConfig *rest.Config
			testScheme := runtime.NewScheme() // new scheme required for client, to add our v1beta1 Infra resources

			fmt.Println("Setting up!")
			ctx = context.TODO()

			fmt.Println("Getting the CAPI YAML!")
			capiYaml, err = os.ReadFile(PathToWorkloadClusterCapiYaml)
			ExpectWithOffset(1, err).NotTo(HaveOccurred(), "Failed to read CAPI YAML")
			ExpectWithOffset(1, capiYaml).NotTo(BeEmpty(), "CAPI YAML is empty")

			fmt.Println("Getting the Kubernetes Config!")
			kubeConfig, err = os.ReadFile(PathToMngmntClusterKubecfg)
			ExpectWithOffset(1, err).NotTo(HaveOccurred(), "Failed to read Kubernetes Config")
			ExpectWithOffset(1, kubeConfig).NotTo(BeEmpty(), "Kubernetes Config is empty")

			restConfig, cs, err = utils.CreateClientConfig(kubeConfig)
			ExpectWithOffset(1, err).NotTo(HaveOccurred(), "Failed to create client config")

			utilruntime.Must(scheme.AddToScheme(testScheme))
			utilruntime.Must(clusterv1beta1.AddToScheme(testScheme))
			runtimeClient, err = runtimeclient.New(restConfig, runtimeclient.Options{Scheme: testScheme})
			ExpectWithOffset(1, runtimeClient).NotTo(BeNil(), "Failed to create runtime client")
			ExpectWithOffset(1, err).NotTo(HaveOccurred(), "Failed to create runtime client")
		})

		It("CAPVCD should be able to create the workload cluster", func() {
			var err error
			By("Getting the clusterName and namespace")
			clusterName, clusterNameSpace, err = utils.GetClusterNameAndNamespaceFromCAPIYaml(capiYaml)
			ExpectWithOffset(1, err).NotTo(HaveOccurred(), "Failed to get cluster name and namespace")
			ExpectWithOffset(1, clusterName).NotTo(BeEmpty(), "Cluster name is empty")
			ExpectWithOffset(1, clusterNameSpace).NotTo(BeEmpty(), "Cluster namespace is empty")

			By("Creating the namespace when necessary")
			err = utils.CreateOrGetNameSpace(ctx, cs, clusterNameSpace)
			ExpectWithOffset(1, err).NotTo(HaveOccurred(), "Failed to create or get namespace")

			By("Applying the CAPI YAML in the CAPVCD mgmt cluster")
			err = utils.ApplyCAPIYaml(ctx, runtimeClient, capiYaml)
			ExpectWithOffset(1, err).NotTo(HaveOccurred(), "Failed to apply CAPI YAML")

			err = utils.WaitForClusterProvisioned(ctx, runtimeClient, clusterName, clusterNameSpace)
			ExpectWithOffset(1, err).NotTo(HaveOccurred(), "Failed to wait for cluster provisioned")

			By("Retrieving the kube config of the workload cluster")
			kubeConfigBytes, err := kcfg.FromSecret(ctx, runtimeClient, runtimeclient.ObjectKey{
				Namespace: clusterNameSpace,
				Name:      clusterName,
			})
			ExpectWithOffset(1, err).NotTo(HaveOccurred(), "Failed to retrieve kube config")
			ExpectWithOffset(1, kubeConfigBytes).NotTo(BeNil(), "Kube config is nil")

			By("Validating the kube config of the workload cluster")
			workloadClientSet, err = utils.ValidateKubeConfig(ctx, kubeConfigBytes)
			ExpectWithOffset(1, err).NotTo(HaveOccurred(), "Failed to validate kube config")
			ExpectWithOffset(1, workloadClientSet).NotTo(BeNil(), "Workload client set is nil")

			By("Getting the vcdcluster of the CAPVCD cluster - management cluster")
			vcdCluster, err = utils.GetVCDClusterFromCluster(ctx, cs, clusterNameSpace, clusterName)
			ExpectWithOffset(1, err).NotTo(HaveOccurred(), "Failed to get VCDCluster from the cluster")
			ExpectWithOffset(1, vcdCluster).NotTo(BeNil(), "VCDCluster is nil")

			By("Getting the rdeID from the vcdCluster")
			rdeID = vcdCluster.Status.InfraId
			ExpectWithOffset(1, rdeID).NotTo(BeZero(), "rdeID is empty")

			By("Creating the cluster resource sets")
			err = utils.CreateWorkloadClusterResources(ctx, workloadClientSet, string(capiYaml), rdeID, vcdCluster)
			ExpectWithOffset(1, err).NotTo(HaveOccurred(), "Failed to create cluster Resource Set")

			By("Waiting for the cluster machines to become running state")
			err = utils.WaitForMachinesRunning(ctx, runtimeClient, clusterName, clusterNameSpace)
			ExpectWithOffset(1, err).NotTo(HaveOccurred(), "Failed to wait for machines running")
		})

		It("CAPVCD should be able to resize the cluster", func() {
			var err error
			testClient, err = utils.ConstructCAPVCDTestClient(ctx, vcdCluster, workloadClientSet, string(capiYaml), clusterNameSpace, clusterName, rdeID, true)
			ExpectWithOffset(1, err).NotTo(HaveOccurred(), "Failed to construct CAPVCD test client")
			ExpectWithOffset(1, testClient).NotTo(BeNil(), "Test client is nil")

			By("Setting desired worker pool node account")
			workerPoolNodes, err := testClient.GetWorkerNodes(context.Background())
			ExpectWithOffset(1, err).NotTo(HaveOccurred(), "Failed to get worker nodes")
			ExpectWithOffset(1, workerPoolNodes).NotTo(BeNil(), "Worker nodes are nil")

			initialNodeCount := len(workerPoolNodes)
			desiredWorkerNodeCount = int64(initialNodeCount + 2)

			By("Resizing the node pool in the workload cluster")
			err = utils.ScaleNodePool(ctx, runtimeClient, desiredWorkerNodeCount, capiYaml)
			ExpectWithOffset(1, err).NotTo(HaveOccurred(), "Failed to scale node pool")

			By(fmt.Sprintf("Monitoring the status of machines inside the CAPVCD cluster after increasing the node pool from %d to %d", initialNodeCount, len(workerPoolNodes)))
			err = utils.MonitorK8sNodePools(testClient, runtimeClient, desiredWorkerNodeCount)
			ExpectWithOffset(1, err).NotTo(HaveOccurred(), "Failed to monitor K8s node pools")

			By(fmt.Sprintf("Verifying the final number of worker nodes after increasing the node pool to %d", len(workerPoolNodes)))
			workerPoolNodes, err = testClient.GetWorkerNodes(context.Background())
			ExpectWithOffset(1, err).NotTo(HaveOccurred(), "Failed to get worker nodes after resizing")
			ExpectWithOffset(1, len(workerPoolNodes)).To(Equal(int(desiredWorkerNodeCount)), "Unexpected number of worker nodes after resizing")

			initialNodeCount = int(desiredWorkerNodeCount)
			desiredWorkerNodeCount = int64(len(workerPoolNodes) - 2)

			By("Resizing the node pool in the workload cluster")
			err = utils.ScaleNodePool(ctx, runtimeClient, desiredWorkerNodeCount, capiYaml)
			ExpectWithOffset(1, err).NotTo(HaveOccurred(), "Failed to scale node pool")

			By(fmt.Sprintf("Monitoring the status of machines inside the CAPVCD cluster after decreasing the node pool from %d to %d", initialNodeCount, desiredWorkerNodeCount))
			err = utils.MonitorK8sNodePools(testClient, runtimeClient, desiredWorkerNodeCount)
			ExpectWithOffset(1, err).NotTo(HaveOccurred(), "Failed to monitor K8s node pools")

			By(fmt.Sprintf("Verifying the final number of worker nodes after decreasing the node pool to %d", len(workerPoolNodes)))
			workerPoolNodes, err = testClient.GetWorkerNodes(context.Background())
			ExpectWithOffset(1, err).NotTo(HaveOccurred(), "Failed to get worker nodes after resizing")
			ExpectWithOffset(1, len(workerPoolNodes)).To(Equal(int(desiredWorkerNodeCount)), "Unexpected number of worker nodes after resizing")
		})

		It("CAPVCD should be able to delete the workload cluster", func() {
			By("Deleting the workload cluster")
			err := utils.DeleteCAPIYaml(ctx, runtimeClient, capiYaml)
			ExpectWithOffset(1, err).NotTo(HaveOccurred(), "Failed to delete CAPI YAML")

			By("Monitoring the cluster get deleted")
			err = utils.WaitForClusterDelete(ctx, runtimeClient, clusterName, clusterNameSpace)
			ExpectWithOffset(1, err).NotTo(HaveOccurred(), "Failed to monitor cluster deletion")
			err = testClient.DeleteNameSpace(ctx, clusterName)
			ExpectWithOffset(1, err).NotTo(HaveOccurred(), "Failed to delete cluster namespace")
			By("Checking the RDE entity of the cluster is already deleted")
			err = utils.CheckRdeEntityNonExisted(ctx, testClient.VcdClient, rdeID, clusterName)
			ExpectWithOffset(1, err).NotTo(HaveOccurred(), "Failed to delete the cluster RDE entity")
		})
	})
})
