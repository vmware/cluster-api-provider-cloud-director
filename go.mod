module github.com/vmware/cluster-api-provider-cloud-director

go 1.16

require (
	github.com/antihax/optional v1.0.0
	github.com/apparentlymart/go-cidr v1.1.0
	github.com/go-openapi/errors v0.20.0
	github.com/google/uuid v1.3.0
	github.com/onsi/ginkgo v1.16.4
	github.com/onsi/gomega v1.15.0
	github.com/peterhellberg/link v1.1.0
	github.com/pkg/errors v0.9.1
	github.com/replicatedhq/troubleshoot v0.25.0
	github.com/stretchr/testify v1.7.0
	github.com/vmware/go-vcloud-director/v2 v2.12.0-alpha.4
	golang.org/x/oauth2 v0.0.0-20210628180205-a41e5a781914
	gopkg.in/yaml.v2 v2.4.0
	k8s.io/api v0.22.4
	k8s.io/apimachinery v0.22.4
	k8s.io/client-go v0.22.4
	k8s.io/klog v1.0.0
	k8s.io/klog/v2 v2.9.0
	sigs.k8s.io/cluster-api v0.4.1
	sigs.k8s.io/controller-runtime v0.10.3
	sigs.k8s.io/kind v0.11.1
)
