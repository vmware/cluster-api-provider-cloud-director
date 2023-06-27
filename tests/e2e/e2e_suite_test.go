package e2e

import (
	"flag"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	"testing"
)

var (
	PathToMngmntClusterKubecfg    string
	PathToWorkloadClusterCapiYaml string
)

func init() {
	flag.StringVar(&PathToMngmntClusterKubecfg, "PathToMngmntClusterKubecfg", "", "path to find the Kubeconfig. It is used to retrieve the cluster")
	flag.StringVar(&PathToWorkloadClusterCapiYaml, "PathToWorkloadClusterCapiYaml", "", "path to find the capi Yaml. It is used to generate the vcd cluster")
}

var _ = BeforeSuite(func() {

	Expect(PathToMngmntClusterKubecfg).NotTo(BeZero(), "Please make sure --PathToMngmntClusterKubecfg is set correctly.")
	Expect(PathToWorkloadClusterCapiYaml).NotTo(BeZero(), "Please make sure --PathToWorkloadClusterCapiYaml is set correctly.")
})

func TestCAPVCDAutomation(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "CAPVCD Testing Suite")
}
