/*
Copyright 2021 The Everoute Authors.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package policy_test

import (
	"testing"
	"time"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/rand"

	"github.com/everoute/everoute/pkg/client/clientset_generated/clientset"
	"github.com/everoute/everoute/pkg/client/clientset_generated/clientset/fake"
	"github.com/everoute/everoute/pkg/client/informers_generated/externalversions"
	controller "github.com/everoute/everoute/plugin/tower/pkg/controller/policy"
	"github.com/everoute/everoute/plugin/tower/pkg/informer"
	fakeserver "github.com/everoute/everoute/plugin/tower/pkg/server/fake"
)

var (
	crdClient       clientset.Interface
	server          *fakeserver.Server
	namespace       = metav1.NamespaceDefault
	podNamespace    = metav1.NamespaceSystem
	stopCh          = make(chan struct{})
	everouteCluster = rand.String(10)
)

const (
	timeout  = time.Second * 10
	interval = time.Millisecond * 250
)

func TestPolicyController(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "PolicyController Suite")
}

var _ = BeforeSuite(func() {
	By("create fake server and fake client")
	server = fakeserver.NewServer(nil)
	server.Serve()
	crdClient = fake.NewSimpleClientset()

	towerFactory := informer.NewSharedInformerFactory(server.NewClient(), 0)
	crdFactory := externalversions.NewSharedInformerFactory(crdClient, 0)

	By("create and start PolicyController")
	controller := controller.New(towerFactory, crdFactory, crdClient, 0, namespace, podNamespace, everouteCluster)
	go controller.Run(10, stopCh)

	By("start towerFactory and crdFactory")
	towerFactory.Start(stopCh)
	crdFactory.Start(stopCh)

	By("wait for tower cache and crd cache sync")
	crdFactory.WaitForCacheSync(stopCh)
	towerFactory.WaitForCacheSync(stopCh)
}, 60)

var _ = AfterSuite(func() {
	By("tearing down the environment")
	close(stopCh)
})
