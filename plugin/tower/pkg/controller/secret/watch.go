package secret

import (
	"context"
	"fmt"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/klog/v2"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/event"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/predicate"

	"github.com/everoute/everoute/pkg/constants"
	ersource "github.com/everoute/everoute/pkg/source"
)

type Watch struct {
	Queue             chan event.GenericEvent
	SKSKubeconfigName string
	SKSKubeconfigNs   string
}

func (w *Watch) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	log := ctrl.LoggerFrom(ctx)
	log.Info("Reconcile start")

	obj := ersource.Event{}
	obj.Name = constants.TowerSKSKubeconfigName
	obj.Namespace = constants.TowerSKSKubeconfigNs
	w.Queue <- event.GenericEvent{Object: &obj}
	log.Info("Reconcile end")
	return ctrl.Result{}, nil
}

func (w *Watch) SetupWithManager(mgr ctrl.Manager, platForm string) error {
	ctrlName := platForm + "-secret-watch"
	if mgr == nil {
		klog.Errorf("Can't setup with nil manager for %s controller", ctrlName)
		return fmt.Errorf("can't setup with nil manager")
	}

	if w.Queue == nil {
		klog.Errorf("Can't setup with nil Queue for %s controller", ctrlName)
		return fmt.Errorf("can't setup with nil Queue for secret-watch controller")
	}
	c, err := controller.New(ctrlName, mgr, controller.Options{
		Reconciler: w,
	})
	if err != nil {
		klog.Errorf("Failed to new %s controller: %s", ctrlName, err)
		return err
	}
	err = c.Watch(ersource.Kind(mgr.GetCache(), &corev1.Secret{}), &handler.EnqueueRequestForObject{}, predicate.Funcs{
		CreateFunc: func(ce event.CreateEvent) bool {
			return w.isSKSKubeconfig(ce.Object)
		},
		UpdateFunc: func(ue event.UpdateEvent) bool {
			return w.isSKSKubeconfig(ue.ObjectNew)
		},
		DeleteFunc: func(de event.DeleteEvent) bool {
			return w.isSKSKubeconfig(de.Object)
		},
	})
	if err != nil {
		klog.Errorf("Controller er-secret failed to watch secret: %s", err)
		return err
	}
	return nil
}

func (w *Watch) isSKSKubeconfig(obj client.Object) bool {
	return obj.GetName() == w.SKSKubeconfigName && obj.GetNamespace() == w.SKSKubeconfigNs
}
