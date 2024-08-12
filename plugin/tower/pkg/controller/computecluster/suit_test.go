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

package computecluster

import (
	"context"
	"testing"
	"time"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	"k8s.io/apimachinery/pkg/util/rand"
	k8sinformers "k8s.io/client-go/informers"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/kubernetes/fake"

	"github.com/everoute/everoute/plugin/tower/pkg/informer"
	fakeserver "github.com/everoute/everoute/plugin/tower/pkg/server/fake"
)

var (
	erClient        kubernetes.Interface
	server          *fakeserver.Server
	ctx, cancel     = context.WithCancel(context.Background())
	everouteCluster = rand.String(10)
	towerSpace      = "tower-space"
	controller      *Controller
)

const (
	timeout  = time.Second * 10
	interval = time.Millisecond * 250
)

func TestELFidController(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "ELFController Suite")
}

var _ = BeforeSuite(func() {
	By("create fake server and fake client")
	server = fakeserver.NewServer(nil)
	server.Serve()
	erClient = fake.NewSimpleClientset()

	towerFactory := informer.NewSharedInformerFactory(server.NewClient(), 0)
	erFactory := k8sinformers.NewSharedInformerFactoryWithOptions(erClient, 0, k8sinformers.WithNamespace(towerSpace))

	By("create elfController")
	controller = &Controller{
		EverouteClusterID:  everouteCluster,
		ConfigmapNamespace: towerSpace,
	}
	controller.Setup(towerFactory, erFactory, erClient)

	By("start towerFactory and erFactory")
	towerFactory.Start(ctx.Done())
	erFactory.Start(ctx.Done())

	By("wait for tower cache and er cache sync")
	erFactory.WaitForCacheSync(ctx.Done())
	towerFactory.WaitForCacheSync(ctx.Done())

	By("start elfController")
	go controller.Run(ctx)
}, 60)

var _ = AfterSuite(func() {
	By("tearing down the environment")
	cancel()
})
