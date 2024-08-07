package secret

import (
	"fmt"

	"github.com/everoute/everoute/pkg/constants"
	"github.com/golang/mock/gomock"
	_ "github.com/golang/mock/mockgen/model"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/event"

	ersource "github.com/everoute/everoute/pkg/source"
	"github.com/everoute/everoute/tests/mocks"
)

var _ = Describe("secret-process", func() {
	towerKey := types.NamespacedName{
		Namespace: constants.TowerSKSKubeconfigNs,
		Name:      constants.TowerSKSKubeconfigName,
	}
	erKey := types.NamespacedName{
		Namespace: constants.SKSKubeconfigNs,
		Name:      constants.SKSKubeconfigName,
	}
	ev := ersource.Event{}
	ev.Name = constants.TowerSKSKubeconfigName
	ev.Namespace = constants.TowerSKSKubeconfigNs

	BeforeEach(func() {
		pCtrl.ERCli = k8sClient
		pCtrl.TowerCli = k8sClient
		Eventually(func(g Gomega) {
			ns := corev1.Namespace{}
			g.Expect(k8sClient.Get(ctx, types.NamespacedName{Name: constants.TowerSKSKubeconfigNs}, &ns)).Should(Succeed())
		})
	})

	AfterEach(func() {
		Eventually(func(g Gomega) {
			for _, c := range []types.NamespacedName{towerKey, erKey} {
				obj := &corev1.Secret{}
				err := k8sClient.Get(ctx, c, obj)
				if err == nil {
					if len(obj.ObjectMeta.Finalizers) != 0 {
						obj.ObjectMeta.Finalizers = []string{}
						g.Expect(k8sClient.Update(ctx, obj)).Should(Succeed())
					}
					obj := &corev1.Secret{}
					err2 := k8sClient.Get(ctx, c, obj)
					if err2 == nil {
						g.Expect(k8sClient.Delete(ctx, obj)).Should(Succeed())
					}
				}
				obj = &corev1.Secret{}
				g.Expect(apierrors.IsNotFound(k8sClient.Get(ctx, c, obj))).Should(BeTrue())
			}
		}, timeout, interval).Should(Succeed())
	})

	Context("create", func() {
		BeforeEach(func() {
			towerSecret := corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{
					Name:      constants.TowerSKSKubeconfigName,
					Namespace: constants.TowerSKSKubeconfigNs,
				},
				Data: make(map[string][]byte, 1),
			}
			towerSecret.Data["value"] = []byte("test")
			Expect(k8sClient.Create(ctx, &towerSecret)).Should(Succeed())
		})

		It("add success", func() {
			queue <- event.GenericEvent{Object: &ev}
			Eventually(func(g Gomega) {
				obj := corev1.Secret{}
				g.Expect(k8sClient.Get(ctx, erKey, &obj)).Should(Succeed())
				g.Expect(obj.ObjectMeta.Finalizers).Should(ContainElement(constants.SKSKubeconfigFinalizer))
				g.Expect(obj.Data).Should(HaveKeyWithValue("value", []byte("test")))
			}, timeout, interval).Should(Succeed())
		})

		It("add failed for tower secret is invalid", func() {
			towerObj := corev1.Secret{}
			Eventually(func(g Gomega) {
				g.Expect(k8sClient.Get(ctx, towerKey, &towerObj)).Should(Succeed())
				towerObj.Data = make(map[string][]byte)
				g.Expect(k8sClient.Update(ctx, &towerObj)).Should(Succeed())
				g.Expect(k8sClient.Get(ctx, towerKey, &towerObj)).Should(Succeed())
				g.Expect(towerObj.Data["value"]).Should(BeNil())
			}, timeout, interval).Should(Succeed())
			_, err := pCtrl.Reconcile(ctx, ctrl.Request{NamespacedName: towerKey})
			Expect(err).ShouldNot(BeNil())
			Expect(err.Error()).Should(Equal("sks kubeconfig in tower is invalid"))
		})

		It("add failed for ercli get failed", func() {
			resErr := fmt.Errorf("unexpect get err")
			mockCli := mocks.NewMockClient(mockCtl)
			mockCli.EXPECT().Get(ctx, gomock.Any(), gomock.Any()).Return(resErr)
			pCtrl.ERCli = mockCli
			_, err := pCtrl.Reconcile(ctx, ctrl.Request{NamespacedName: towerKey})
			Expect(err).Should(Equal(resErr))
		})

		It("add failed for ercli create failed", func() {
			resErr := fmt.Errorf("unexpect create err")
			mockCli := mocks.NewMockClient(mockCtl)
			mockCli.EXPECT().Get(ctx, gomock.Any(), gomock.Any()).DoAndReturn(k8sClient.Get)
			mockCli.EXPECT().Create(ctx, gomock.Any()).Return(resErr)
			pCtrl.ERCli = mockCli
			_, err := pCtrl.Reconcile(ctx, ctrl.Request{NamespacedName: towerKey})
			Expect(err).Should(Equal(resErr))
		})
	})

	Context("update", func() {
		BeforeEach(func() {
			towerSecret := corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{
					Name:      constants.TowerSKSKubeconfigName,
					Namespace: constants.TowerSKSKubeconfigNs,
				},
				Data: make(map[string][]byte, 1),
			}
			towerSecret.Data["value"] = []byte("test-update")
			Expect(k8sClient.Create(ctx, &towerSecret)).Should(Succeed())

			erSecret := corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{
					Name:       constants.SKSKubeconfigName,
					Namespace:  constants.SKSKubeconfigNs,
					Finalizers: []string{constants.SKSKubeconfigFinalizer},
				},
				Data: make(map[string][]byte),
			}
			erSecret.Data["value"] = []byte("test")
			Expect(k8sClient.Create(ctx, &erSecret)).Should(Succeed())
		})

		It("update success", func() {
			queue <- event.GenericEvent{Object: &ev}
			Eventually(func(g Gomega) {
				obj := corev1.Secret{}
				g.Expect(k8sClient.Get(ctx, erKey, &obj)).Should(Succeed())
				g.Expect(obj.ObjectMeta.Finalizers).Should(ContainElement(constants.SKSKubeconfigFinalizer))
				g.Expect(obj.Data).Should(HaveKeyWithValue("value", []byte("test-update")))
			}, timeout, interval).Should(Succeed())
		})

		It("update failed for ercli update failed", func() {
			resErr := fmt.Errorf("unexpect update err")
			mockCli := mocks.NewMockClient(mockCtl)
			mockCli.EXPECT().Update(ctx, gomock.Any()).Return(resErr)
			mockCli.EXPECT().Get(ctx, gomock.Any(), gomock.Any()).DoAndReturn(k8sClient.Get)
			pCtrl.ERCli = mockCli
			_, err := pCtrl.Reconcile(ctx, ctrl.Request{NamespacedName: towerKey})
			Expect(err).ShouldNot(BeNil())
			Expect(err).Should(Equal(resErr))
		})
	})

	Context("delete", func() {
		BeforeEach(func() {
			erSecret := corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{
					Name:       constants.SKSKubeconfigName,
					Namespace:  constants.SKSKubeconfigNs,
					Finalizers: []string{constants.SKSKubeconfigFinalizer},
				},
				Data: make(map[string][]byte),
			}
			erSecret.Data["value"] = []byte("test")
			Expect(k8sClient.Create(ctx, &erSecret)).Should(Succeed())
		})
		It("delete success for tower secret not found", func() {
			queue <- event.GenericEvent{Object: &ev}
			Eventually(func(g Gomega) {
				obj := corev1.Secret{}
				err := k8sClient.Get(ctx, erKey, &obj)
				g.Expect(err).Should(BeNil())
				g.Expect(obj.ObjectMeta.DeletionTimestamp).ShouldNot(BeNil())
			}, timeout, interval).Should(Succeed())
		})

		It("delete success for tower secret DeletionTimestamp", func() {
			towerSecret := corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{
					Name:       constants.TowerSKSKubeconfigName,
					Namespace:  constants.TowerSKSKubeconfigNs,
					Finalizers: []string{"test.io/temp"},
				},
				Data: make(map[string][]byte, 1),
			}
			towerSecret.Data["value"] = []byte("test")
			Expect(k8sClient.Create(ctx, &towerSecret)).Should(Succeed())
			Expect(k8sClient.Delete(ctx, &towerSecret)).Should(Succeed())
			Eventually(func(g Gomega) {
				obj := corev1.Secret{}
				err := k8sClient.Get(ctx, towerKey, &obj)
				g.Expect(err).Should(BeNil())
				g.Expect(obj.ObjectMeta.DeletionTimestamp).ShouldNot(BeNil())
			})

			queue <- event.GenericEvent{Object: &ev}
			Eventually(func(g Gomega) {
				obj := corev1.Secret{}
				err := k8sClient.Get(ctx, erKey, &obj)
				g.Expect(err).Should(BeNil())
				g.Expect(obj.ObjectMeta.DeletionTimestamp).ShouldNot(BeNil())
			}, timeout, interval).Should(Succeed())
		})

		It("delete success for ercli not found err", func() {
			resErr := fmt.Errorf("unexpect delete err")
			mockCli := mocks.NewMockClient(mockCtl)
			mockCli.EXPECT().Get(ctx, gomock.Any(), gomock.Any()).Return(&apierrors.StatusError{ErrStatus: metav1.Status{Reason: metav1.StatusReasonNotFound}})
			mockCli.EXPECT().Delete(ctx, gomock.Any()).Return(resErr).Times(0)
			pCtrl.ERCli = mockCli
			_, err := pCtrl.Reconcile(ctx, ctrl.Request{NamespacedName: towerKey})
			Expect(err).Should(BeNil())
		})

		It("delete failed to ercli get failed", func() {
			resErr := fmt.Errorf("unexpect get err")
			mockCli := mocks.NewMockClient(mockCtl)
			mockCli.EXPECT().Get(ctx, gomock.Any(), gomock.Any()).Return(resErr)
			pCtrl.ERCli = mockCli
			_, err := pCtrl.Reconcile(ctx, ctrl.Request{NamespacedName: towerKey})
			Expect(err).Should(Equal(resErr))
		})

		It("delete failed for ercli delete failed", func() {
			resErr := fmt.Errorf("unexpect delete err")
			mockCli := mocks.NewMockClient(mockCtl)
			mockCli.EXPECT().Get(ctx, gomock.Any(), gomock.Any()).DoAndReturn(k8sClient.Get)
			mockCli.EXPECT().Delete(ctx, gomock.Any()).Return(resErr)
			pCtrl.ERCli = mockCli
			_, err := pCtrl.Reconcile(ctx, ctrl.Request{NamespacedName: towerKey})
			Expect(err).Should(Equal(resErr))
		})
	})

	Context("other err", func() {
		It("get secret from tower failed", func() {
			resErr := fmt.Errorf("unexpect get err")
			mockCli := mocks.NewMockClient(mockCtl)
			mockCli.EXPECT().Get(ctx, gomock.Any(), gomock.Any()).Return(resErr)
			pCtrl.TowerCli = mockCli
			_, err := pCtrl.Reconcile(ctx, ctrl.Request{NamespacedName: towerKey})
			Expect(err).Should(Equal(resErr))
		})

		It("param err", func() {
			mockCli := mocks.NewMockClient(mockCtl)
			mockCli.EXPECT().Get(ctx, gomock.Any(), gomock.Any()).Times(0)
			pCtrl.TowerCli = mockCli

			_, err := pCtrl.Reconcile(ctx, ctrl.Request{NamespacedName: erKey})
			Expect(err).Should(BeNil())
		})
	})
})
