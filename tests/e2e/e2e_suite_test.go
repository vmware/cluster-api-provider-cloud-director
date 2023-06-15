package e2e

import (
	"flag"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	"testing"
)

var (
	kubeCfgPath  string
	capiYamlPath string
	rdeId        string
	host         string
	org          string
	ovdc         string
	ovdcNetwork  string
	userName     string
	userOrg      string
	refreshToken string
)

func init() {
	//Inputs needed: VCD site, org, ovdc, username, refreshToken, clusterId
	flag.StringVar(&kubeCfgPath, "kubeCfgPath", "", "path to find the Kubeconfig. It is used to retrieve the cluster")
	flag.StringVar(&capiYamlPath, "capiYamlPath", "", "path to find the capi Yaml. It is used to generate the vcd cluster")
	flag.StringVar(&host, "host", "", "VCD host site to generate client")
	flag.StringVar(&org, "org", "", "Cluster Org to generate client")
	flag.StringVar(&userOrg, "userOrg", "", "User Org to generate client")
	flag.StringVar(&ovdc, "ovdc", "", "Ovdc Name to generate client")
	flag.StringVar(&ovdcNetwork, "ovdcNetwork", "", "Ovdc Network Name to generate client")
	flag.StringVar(&userName, "userName", "", "Username for login to generate client")
	flag.StringVar(&refreshToken, "refreshToken", "", "Refresh token of user to generate client")
}

var _ = BeforeSuite(func() {

	Expect(kubeCfgPath).NotTo(BeZero(), "Please make sure --kubeCfgPath is set correctly.")
	Expect(capiYamlPath).NotTo(BeZero(), "Please make sure --capiYamlPath is set correctly.")
	Expect(host).NotTo(BeZero(), "Please make sure --host is set correctly.")
	Expect(org).NotTo(BeZero(), "Please make sure --org is set correctly.")
	Expect(userOrg).NotTo(BeZero(), "Please make sure --userOrg is set correctly.")
	Expect(ovdc).NotTo(BeZero(), "Please make sure --ovdc is set correctly.")
	Expect(ovdcNetwork).NotTo(BeZero(), "Please make sure --ovdcNetwork is set correctly.")
	Expect(userName).NotTo(BeZero(), "Please make sure --userName is set correctly.")
	Expect(refreshToken).NotTo(BeZero(), "Please make sure --refreshToken is set correctly.")
})

func TestCSIAutomation(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "CAPVCD Testing Suite")
}
