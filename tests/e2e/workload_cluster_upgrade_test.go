package e2e

import (
	"context"
	"encoding/json"
	"fmt"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	"github.com/vmware/cloud-provider-for-cloud-director/pkg/testingsdk"
	infrav2 "github.com/vmware/cluster-api-provider-cloud-director/api/v1beta2"
	infrav1beta3 "github.com/vmware/cluster-api-provider-cloud-director/api/v1beta3"
	"github.com/vmware/cluster-api-provider-cloud-director/tests/e2e/utils"
	"github.com/vmware/go-vcloud-director/v2/types/v56"
	"io/ioutil"
	"k8s.io/apimachinery/pkg/runtime"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	"k8s.io/apimachinery/pkg/version"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/rest"
	"os"
	clusterv1beta1 "sigs.k8s.io/cluster-api/api/v1beta1"
	kcfg "sigs.k8s.io/cluster-api/util/kubeconfig"
	runtimeclient "sigs.k8s.io/controller-runtime/pkg/client"
	"strings"
)

const (
	tkgMapJsonPath string = "../assets/tkg_map.json"
)

var _ = Describe("Workload Cluster Life cycle management", func() {
	When("create, upgrade, and delete the workload cluster", func() {
		var (
			runtimeClient     runtimeclient.Client
			testClient        *testingsdk.TestClient
			ctx               context.Context
			kubeConfig        []byte
			capiYaml          []byte
			cs                *kubernetes.Clientset
			workloadClientSet *kubernetes.Clientset
			clusterName       string
			clusterNameSpace  string
			vcdCluster        *infrav1beta3.VCDCluster
			rdeID             string
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
			utilruntime.Must(infrav2.AddToScheme(testScheme))
			runtimeClient, err = runtimeclient.New(restConfig, runtimeclient.Options{Scheme: testScheme})
			ExpectWithOffset(1, runtimeClient).NotTo(BeNil(), "Failed to create runtime client")
			ExpectWithOffset(1, err).NotTo(HaveOccurred(), "Failed to create runtime client")
		})

		It("CAPVCD should create cluster before upgrade", func() {
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
			vcdCluster, err = utils.GetVCDClusterFromCluster(ctx, runtimeClient, clusterNameSpace, clusterName)
			ExpectWithOffset(1, err).NotTo(HaveOccurred(), "Failed to get VCDCluster from the cluster")
			ExpectWithOffset(1, vcdCluster).NotTo(BeNil(), "VCDCluster is nil")

			fmt.Println("Getting the antrea YAML!")
			antreaYamlBytes, err := os.ReadFile(PathToAntreaYaml)
			ExpectWithOffset(1, err).NotTo(HaveOccurred(), "Failed to read Antrea YAML")
			ExpectWithOffset(1, capiYaml).NotTo(BeEmpty(), "Antre YAML is empty")

			By("Installing antera from yaml")
			err = utils.InstallAntreaFromYaml(ctx, kubeConfigBytes, antreaYamlBytes)
			ExpectWithOffset(1, err).NotTo(HaveOccurred(), "Failed to install antrea using antrea yaml")

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

		It("should be able to upgrade successfully given a valid OVA to upgrade to is installed", func() {
			var (
				err               error
				updatedK8sVersion string
				byteValue         []byte
				tkgMapJson        *os.File
				tkgMap            map[string]interface{}
				k8sVersion        *version.Info
				allVappTemplates  []*types.QueryResultVappTemplateType
			)

			testClient, err := utils.ConstructCAPVCDTestClient(ctx, vcdCluster, workloadClientSet, string(capiYaml), clusterNameSpace, clusterName, rdeID, true)
			ExpectWithOffset(1, err).NotTo(HaveOccurred(), "Failed to construct CAPVCD test client")
			ExpectWithOffset(1, testClient).NotTo(BeNil(), "Test client is nil")

			// Gathering all vAppTemplates from every catalog
			allVappTemplates, err = testingsdk.GetVappTemplates(testClient.VcdClient)
			ExpectWithOffset(1, err).NotTo(HaveOccurred(), "Failed to get allVappTemplates")
			ExpectWithOffset(1, allVappTemplates).NotTo(BeNil(), "allVappTemplates is nil")

			// Caching tkg_map.json into memory
			tkgMapJson, err = os.Open(tkgMapJsonPath)
			ExpectWithOffset(1, err).NotTo(HaveOccurred(), "Failed to open tkg_map.json")

			byteValue, err = ioutil.ReadAll(tkgMapJson)
			ExpectWithOffset(1, err).NotTo(HaveOccurred(), "Failed to read tkgMapJson")
			err = json.Unmarshal(byteValue, &tkgMap)
			ExpectWithOffset(1, err).NotTo(HaveOccurred(), "Failed to unmarshal tkgMapJson")

			// Getting the cluster's current k8s version
			k8sVersion, err = testClient.GetK8sVersion()
			ExpectWithOffset(1, err).NotTo(HaveOccurred(), "Failed to get Kubernetes version")
			k8sSplit := strings.Split(k8sVersion.GitVersion, "+")
			k8sVersionStr := k8sSplit[0]

			fmt.Printf("Kubernetes version: %s\n", k8sVersionStr)

			By("upgrading the cluster to an OVA with the next minor version k8s Upgrade")
			updatedK8sVersion, err = utils.UpgradeCluster(ctx, runtimeClient, string(capiYaml), k8sVersionStr, TargetK8SVersionForUpgradeTest, TargetTKGVersionForUpgradeTest, tkgMap, allVappTemplates)
			ExpectWithOffset(1, err).NotTo(HaveOccurred(), "Failed to upgrade the cluster")

			By("confirming all nodes have been upgraded to the new k8s version")
			err = utils.MonitorK8sUpgrade(testClient, runtimeClient, updatedK8sVersion)
			ExpectWithOffset(1, err).NotTo(HaveOccurred(), "Failed to monitor Kubernetes upgrade")
		})

		It("CAPVCD should be able to delete the workload cluster", func() {
			By("Deleting the workload cluster")
			err := utils.DeleteCAPIYaml(ctx, runtimeClient, capiYaml)
			ExpectWithOffset(1, err).NotTo(HaveOccurred(), "Failed to delete CAPI YAML")

			By("Monitoring the cluster get deleted")
			err = utils.WaitForClusterDelete(ctx, runtimeClient, clusterName, clusterNameSpace)
			ExpectWithOffset(1, err).NotTo(HaveOccurred(), "Failed to monitor cluster deletion")

			By("deleting all the resource in the cluster nameSpace")
			// using the kind cluster as the clientSet of testClient
			testClient.Cs = cs
			// Secret(capi-user-credentials) should also be deleted
			err = testClient.DeleteNameSpace(ctx, clusterNameSpace)
			ExpectWithOffset(1, err).NotTo(HaveOccurred(), "Failed to delete cluster namespace")

			By("Checking the RDE entity of the cluster is already deleted")
			err = utils.CheckRdeEntityNonExisted(ctx, testClient.VcdClient, rdeID, clusterName)
			ExpectWithOffset(1, err).NotTo(HaveOccurred(), "Failed to delete the cluster RDE entity")
		})
	})
})
