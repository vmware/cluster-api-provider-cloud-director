package e2e

import (
	"context"
	"fmt"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	"github.com/vmware/cloud-provider-for-cloud-director/pkg/testingsdk"
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

var _ = Describe("Workload Cluster CRUD based the completed management Cluster", func() {
	When("create, resize and delete the workload cluster", func() {
		var (
			runtimeClient          runtimeclient.Client
			testClient             *testingsdk.TestClient
			ctx                    context.Context
			kubeConfig             []byte
			capiYaml               []byte
			cs                     *kubernetes.Clientset
			desiredWorkerNodeCount int64
			clusterName            string
			clusterNameSpace       string
		)

		BeforeEach(func() {
			var (
				err        error
				restConfig *rest.Config
				testScheme *runtime.Scheme
			)

			fmt.Println("setting up!")
			ctx = context.TODO()

			fmt.Println("getting the CAPI yaml!")
			capiYaml, err = os.ReadFile(capiYamlPath)
			Expect(err).NotTo(HaveOccurred())
			Expect(capiYaml).NotTo(BeEmpty())

			fmt.Println("getting the Kubernetes Config!")
			kubeConfig, err = os.ReadFile(kubeCfgPath)
			Expect(err).NotTo(HaveOccurred())
			Expect(kubeConfig).NotTo(BeEmpty())

			restConfig, cs, err = utils.CreateClientConfig(kubeConfig)
			Expect(err).NotTo(HaveOccurred())

			testScheme = runtime.NewScheme() // new scheme required for client, to add our v1beta1 Infra resources
			utilruntime.Must(scheme.AddToScheme(testScheme))
			utilruntime.Must(clusterv1beta1.AddToScheme(testScheme))
			runtimeClient, err = runtimeclient.New(restConfig, runtimeclient.Options{Scheme: testScheme})
			Expect(runtimeClient).NotTo(BeNil())
			Expect(err).NotTo(HaveOccurred())
		})

		It("CAPVCD should be able to handle the workload cluster", func() {
			var err error
			By("getting the clusterName and namespace")
			clusterName, clusterNameSpace, err = utils.GetClusterNameANDNamespaceFromCAPIYaml(capiYaml)
			By("creating the namespace when necessary")

			utils.CreateOrGetNameSpace(ctx, cs, clusterNameSpace)
			Expect(err).NotTo(HaveOccurred())
			Expect(clusterName).NotTo(BeEmpty())
			Expect(clusterNameSpace).NotTo(BeEmpty())

			By("applying the CAPI yaml in the CAPVCD mgmt cluster")
			utils.ApplyCAPIYaml(ctx, cs, capiYaml)

			By("retrieving the kube config of the workload cluster")
			kubeConfigBytes, err := kcfg.FromSecret(ctx, runtimeClient, runtimeclient.ObjectKey{
				Namespace: clusterNameSpace,
				Name:      clusterName,
			})
			Expect(err).NotTo(HaveOccurred())
			Expect(kubeConfigBytes).NotTo(BeNil())

			By("validating the kube config of the workload cluster")
			err = utils.ValidateKubeConfig(ctx, kubeConfigBytes)
			Expect(err).NotTo(HaveOccurred())

			By("getting the rde Id of the CAPVCD cluster - management cluster")
			rdeId, err := utils.GetRDEIdFromVcdCluster(ctx, cs, clusterNameSpace, clusterName)
			By("initializing the test client")
			testClient, err = utils.NewTestClient(
				host, org, userOrg, ovdc, userName, refreshToken, rdeId, true,
			)
			By("setting desired worker pool node account")
			workerpoolNodes, err := testClient.GetWorkerNodes(context.Background())
			Expect(err).NotTo(HaveOccurred())
			Expect(workerpoolNodes).NotTo(BeNil())

			desiredWorkerNodeCount = int64(len(workerpoolNodes)) + 2

			By("resizing the node pool in the workload cluster")
			utils.ScaleNodePool(ctx, cs, desiredWorkerNodeCount, capiYaml)

			By("monitoring the status of machines inside the CAPVCD cluster")
			err = utils.MonitorK8sNodePools(testClient, runtimeClient, desiredWorkerNodeCount)
			Expect(err).NotTo(HaveOccurred())

			By("deleting the workload cluster")
			err = utils.DeleteCAPIYaml(ctx, cs, capiYaml)
			Expect(err).NotTo(HaveOccurred())

		})

	})

})
