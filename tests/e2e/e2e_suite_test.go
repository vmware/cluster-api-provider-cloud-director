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
	TargetTKGVersion              string
	TargetK8SVersion              string
)

func init() {
	flag.StringVar(&PathToMngmntClusterKubecfg, "PathToMngmntClusterKubecfg", "", "path to find the Kubeconfig. It is used to retrieve the cluster")
	flag.StringVar(&PathToWorkloadClusterCapiYaml, "PathToWorkloadClusterCapiYaml", "", "path to find the capi Yaml. It is used to generate the vcd cluster")
	flag.StringVar(&TargetTKGVersion, "TargetTKGVersion", "", "target TKG version")
	flag.StringVar(&TargetK8SVersion, "TargetK8SVersion", "", "target Kubernetes version")
}

var _ = BeforeSuite(func() {

	Expect(PathToMngmntClusterKubecfg).NotTo(BeZero(), "Please make sure --PathToMngmntClusterKubecfg is set correctly.")
	Expect(PathToWorkloadClusterCapiYaml).NotTo(BeZero(), "Please make sure --PathToWorkloadClusterCapiYaml is set correctly.")
})

func TestCAPVCDAutomation(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "CAPVCD Testing Suite")
}
