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
	When("create, resize and delete the workload cluster", func() {
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
			cluster                *clusterv1beta1.Cluster
			vcdCluster             *infrav2.VCDCluster
			rdeId                  string
		)

		const kubeNameSpace = "kube-system"

		BeforeEach(func() {
			var (
				err        error
				restConfig *rest.Config
				testScheme *runtime.Scheme
			)

			fmt.Println("setting up!")
			ctx = context.TODO()

			fmt.Println("getting the CAPI yaml!")
			capiYaml, err = os.ReadFile(PathToWorkloadClusterCapiYaml)
			Expect(err).NotTo(HaveOccurred(), "failed to read CAPI YAML")
			Expect(capiYaml).NotTo(BeEmpty(), "CAPI YAML is empty")

			fmt.Println("getting the Kubernetes Config!")
			kubeConfig, err = os.ReadFile(PathToMngmntClusterKubecfg)
			Expect(err).NotTo(HaveOccurred(), "failed to read Kubernetes Config")
			Expect(kubeConfig).NotTo(BeEmpty(), "Kubernetes Config is empty")

			restConfig, cs, err = utils.CreateClientConfig(kubeConfig)
			Expect(err).NotTo(HaveOccurred(), "failed to create client config")

			testScheme = runtime.NewScheme() // new scheme required for client, to add our v1beta1 Infra resources
			utilruntime.Must(scheme.AddToScheme(testScheme))
			utilruntime.Must(clusterv1beta1.AddToScheme(testScheme))
			runtimeClient, err = runtimeclient.New(restConfig, runtimeclient.Options{Scheme: testScheme})
			Expect(runtimeClient).NotTo(BeNil(), "failed to create runtime client")
			Expect(err).NotTo(HaveOccurred(), "failed to create runtime client")
		})

		AfterEach(func() {
			var err error
			err = testClient.DeleteNameSpace(ctx, clusterName)
			Expect(err).NotTo(HaveOccurred(), "failed to delete cluster namespace")
		})

		It("CAPVCD should be able to create the workload cluster", func() {
			var err error
			//redId
			By("getting the clusterName and namespace")
			clusterName, clusterNameSpace, err = utils.GetClusterNameANDNamespaceFromCAPIYaml(capiYaml)
			Expect(err).NotTo(HaveOccurred(), "failed to get cluster name and namespace")
			Expect(clusterName).NotTo(BeEmpty(), "cluster name is empty")
			Expect(clusterNameSpace).NotTo(BeEmpty(), "cluster namespace is empty")

			By("creating the namespace when necessary")
			err = utils.CreateOrGetNameSpace(ctx, cs, clusterNameSpace)
			Expect(err).NotTo(HaveOccurred(), "failed to create or get namespace")

			By("applying the CAPI yaml in the CAPVCD mgmt cluster")
			cluster, err = utils.ApplyCAPIYaml(ctx, cs, capiYaml)
			Expect(err).NotTo(HaveOccurred(), "failed to apply CAPI YAML")
			Expect(cluster).NotTo(BeNil(), "cluster is nil")

			err = utils.WaitForClusterProvisioned(ctx, runtimeClient, cluster)
			Expect(err).NotTo(HaveOccurred(), "failed to wait for cluster provisioned")

			By("retrieving the kube config of the workload cluster")
			kubeConfigBytes, err := kcfg.FromSecret(ctx, runtimeClient, runtimeclient.ObjectKey{
				Namespace: clusterNameSpace,
				Name:      clusterName,
			})
			Expect(err).NotTo(HaveOccurred(), "failed to retrieve kube config")
			Expect(kubeConfigBytes).NotTo(BeNil(), "kube config is nil")

			By("validating the kube config of the workload cluster")
			workloadClientSet, err = utils.ValidateKubeConfig(ctx, kubeConfigBytes)
			Expect(err).NotTo(HaveOccurred(), "failed to validate kube config")
			Expect(workloadClientSet).NotTo(BeNil(), "workload client set is nil")

			By("getting the vcdcluster of the CAPVCD cluster - management cluster")
			vcdCluster, err = utils.GetVCDClusterFromCluster(ctx, cs, clusterNameSpace, clusterName)
			Expect(err).NotTo(HaveOccurred(), "failed to get VCDCluster from the cluster")
			Expect(vcdCluster).NotTo(BeNil(), "VCDCluster is nil")

			By("setting the rdeID from the vcdCluster")
			rdeId = vcdCluster.Status.InfraId
			Expect(rdeId).NotTo(BeNil(), "rdeId is empty")

			By("creating the cluster resource sets")
			err = utils.CreateWorkloadClusterResources(ctx, workloadClientSet, string(capiYaml), kubeNameSpace, rdeId, vcdCluster)
			Expect(err).NotTo(HaveOccurred(), "failed to create cluster Resource Set")

			By("waiting the cluster machines becoming running state")
			err = utils.WaitForMachinesRunning(ctx, runtimeClient, cluster)
			Expect(err).NotTo(HaveOccurred(), "failed to wait for machines running")
		})

		It("CAPVCD should be able to resize the cluster", func() {
			var err error
			testClient, err = utils.ConstructCAPVCDTestClient(ctx, workloadClientSet, string(capiYaml), clusterNameSpace, clusterName, rdeId, true)

			By("setting desired worker pool node account")
			workerpoolNodes, err := testClient.GetWorkerNodes(context.Background())
			Expect(err).NotTo(HaveOccurred(), "failed to get worker nodes")
			Expect(workerpoolNodes).NotTo(BeNil(), "worker nodes are nil")

			desiredWorkerNodeCount = int64(len(workerpoolNodes)) + 2

			By("resizing the node pool in the workload cluster")
			err = utils.ScaleNodePool(ctx, cs, desiredWorkerNodeCount, capiYaml)
			Expect(err).NotTo(HaveOccurred(), "failed to scale node pool")

			By("monitoring the status of machines inside the CAPVCD cluster")
			err = utils.MonitorK8sNodePools(testClient, runtimeClient, desiredWorkerNodeCount)
			Expect(err).NotTo(HaveOccurred(), "failed to monitor K8s node pools")

		})

		It("CAPVCD should be able to delete the cluster", func() {
			var err error
			By("deleting the workload cluster")
			err = utils.DeleteCAPIYaml(ctx, cs, capiYaml)
			Expect(err).NotTo(HaveOccurred(), "failed to delete CAPI YAML")

			By("monitoring the cluster get deleted")
			err = utils.WaitForClusterDelete(ctx, runtimeClient, cluster)
			Expect(err).NotTo(HaveOccurred(), "failed to monitor cluster deletion")
		})

	})
})
