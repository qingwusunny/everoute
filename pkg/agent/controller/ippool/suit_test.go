package ippool

import (
	"os"
	"path"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	"sigs.k8s.io/controller-runtime/pkg/envtest"
)

const (
	RunTestWithExistingCluster = "TESTING_WITH_EXISTING_CLUSTER"
	GatewayIP = "10.12.12.1"
)

var (
	testEnv *envtest.Environment
)

var _ = BeforeSuite(func() {
	var useExistingCluster = false
	if os.Getenv(RunTestWithExistingCluster) == "true" {
		useExistingCluster = true
	}

	testEnv = &envtest.Environment{
		UseExistingCluster: &useExistingCluster,
		CRDDirectoryPaths: []string{path.Join("..", "..", "..", "..", "deploy", "chart", "templates", "crds")},
	}
	cfg, err := testEnv.Start()

})