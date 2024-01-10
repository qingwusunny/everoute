package ippool

import (
	"context"
	"fmt"
	"sync"

	ipamv1alpha1 "github.com/everoute/ipam/api/ipam/v1alpha1"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/types"
	uerr "k8s.io/apimachinery/pkg/util/errors"
	"k8s.io/apimachinery/pkg/util/sets"
	"k8s.io/client-go/util/workqueue"
	"k8s.io/klog"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/event"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/predicate"
	"sigs.k8s.io/controller-runtime/pkg/source"

	eproxy "github.com/everoute/everoute/pkg/agent/proxy"
	eipt "github.com/everoute/everoute/pkg/agent/proxy/iptables"
)

var ippoolPredicate predicate.Predicate = predicate.Funcs{
	UpdateFunc: func(e event.UpdateEvent) bool {
		old, ok := e.ObjectOld.(*ipamv1alpha1.IPPool)
		if !ok || old == nil {
			klog.Errorf("Failed to transfer old object to ippool resource: %v", e.ObjectOld)
			return false
		}
		new, ok := e.ObjectNew.(*ipamv1alpha1.IPPool)
		if !ok || new == nil {
			klog.Errorf("Failed to transfer new object to ippool resource: %v", e.ObjectNew)
			return false
		}
		return new.Spec.CIDR != old.Spec.CIDR
	},
}

type Reconciler struct {
	client.Client
	IptCtrl   *eipt.OverlayIPtables
	RouteCtrl *eproxy.OverlayRoute

	oldCIDRs map[types.NamespacedName]sets.Set[string]
	cidrLock sync.Mutex
}

func (r *Reconciler) SetupWithManager(mgr ctrl.Manager) error {
	if mgr == nil {
		return fmt.Errorf("can't setup with nil manager")
	}
	if r.IptCtrl == nil {
		return fmt.Errorf("param IptCtrl can't be nil")
	}
	if r.RouteCtrl == nil {
		return fmt.Errorf("param RouteCtrl can't be nil")
	}

	r.oldCIDRs = make(map[types.NamespacedName]sets.Set[string])

	c, err := controller.New("ippool-ctrl", mgr, controller.Options{
		Reconciler: r,
	})
	if err != nil {
		return err
	}

	return c.Watch(source.Kind(mgr.GetCache(), &ipamv1alpha1.IPPool{}), &handler.Funcs{
		CreateFunc: func(_ context.Context, e event.CreateEvent, q workqueue.RateLimitingInterface) {
			q.Add(ctrl.Request{
				NamespacedName: types.NamespacedName{
					Namespace: e.Object.GetNamespace(),
					Name:      e.Object.GetName(),
				},
			})
		},
		UpdateFunc: r.handleUpdateEvent,
		DeleteFunc: r.handleDeleteEvent,
	}, ippoolPredicate)
}

func (r *Reconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	klog.Infof("Receive ippool %v reconcile", req.NamespacedName)
	pool := ipamv1alpha1.IPPool{}
	err := r.Get(ctx, req.NamespacedName, &pool)
	if err != nil {
		if errors.IsNotFound(err) {
			if err := r.deleteIPPoolOldCIDRs(req.NamespacedName); err != nil {
				klog.Errorf("Failed to delete ippool %v oldcidrs, err: %v", req.NamespacedName, err)
			}
			return ctrl.Result{}, err
		}
		klog.Errorf("Failed to get ippool %v, err: %v", req.NamespacedName, err)
		return ctrl.Result{}, err
	}

	if err := r.deleteIPPoolOldCIDRs(req.NamespacedName); err != nil {
		klog.Errorf("Failed to delete ippool %v oldcidrs, err: %v", req.NamespacedName, err)
		return ctrl.Result{}, err
	}

	if err := r.addIPPoolNewCIDRs(req.NamespacedName, pool.Spec.CIDR); err != nil {
		klog.Errorf("Failed to add ippool %v cidrs, err: %v", pool, err)
		return ctrl.Result{}, err
	}

	return ctrl.Result{}, nil
}

func (r *Reconciler) handleUpdateEvent(_ context.Context, e event.UpdateEvent, q workqueue.RateLimitingInterface) {
	old, ok := e.ObjectOld.(*ipamv1alpha1.IPPool)
	if !ok || old == nil {
		klog.Errorf("Failed to transfer old object to ippool resource: %v", e.ObjectOld)
		return
	}
	nsName := types.NamespacedName{
		Namespace: old.GetNamespace(),
		Name:      old.GetName(),
	}
	if old.Spec.CIDR != "" {
		r.insertOldCIDRsByKey(nsName, old.Spec.CIDR)
	}

	q.Add(ctrl.Request{
		NamespacedName: nsName,
	})
}

func (r *Reconciler) handleDeleteEvent(_ context.Context, e event.DeleteEvent, q workqueue.RateLimitingInterface) {
	o, ok := e.Object.(*ipamv1alpha1.IPPool)
	if !ok || o == nil {
		klog.Errorf("Failed to transfer deleted object to ippool resource: %v", e.Object)
		return
	}
	nsName := types.NamespacedName{
		Namespace: o.GetNamespace(),
		Name:      o.GetName(),
	}
	if o.Spec.CIDR != "" {
		r.insertOldCIDRsByKey(nsName, o.Spec.CIDR)
	}

	q.Add(ctrl.Request{
		NamespacedName: nsName,
	})
}

func (r *Reconciler) insertOldCIDRsByKey(k types.NamespacedName, cidrs ...string) {
	r.cidrLock.Lock()
	defer r.cidrLock.Unlock()

	v, ok := r.oldCIDRs[k]
	if !ok {
		v = sets.New[string]()
	}
	_ = v.Insert(cidrs...)
	r.oldCIDRs[k] = v
}

func (r *Reconciler) deleteOldCIDRsByKey(k types.NamespacedName, cidrs ...string) {
	r.cidrLock.Lock()
	defer r.cidrLock.Unlock()

	v, ok := r.oldCIDRs[k]
	if !ok {
		return
	}
	v.Delete(cidrs...)
	if v.Len() == 0 {
		delete(r.oldCIDRs, k)
		return
	}
	r.oldCIDRs[k] = v
}

func (r *Reconciler) getOldCIDRsByKey(k types.NamespacedName) []string {
	r.cidrLock.Lock()
	defer r.cidrLock.Unlock()

	cidr, ok := r.oldCIDRs[k]
	if !ok {
		return []string{}
	}
	return cidr.UnsortedList()
}

func (r *Reconciler) deleteIPPoolOldCIDRs(k types.NamespacedName) error {
	cidrs := r.getOldCIDRsByKey(k)
	if len(cidrs) == 0 {
		return nil
	}
	var errs []error
	r.IptCtrl.DelPodCIDRs(cidrs...)
	for _, c := range cidrs {
		if err := r.IptCtrl.DelRuleByCIDR(c); err != nil {
			klog.Errorf("Failed to add iptables rule for ippool old cidr %s: %v", c, err)
			errs = append(errs, err)
		}
	}

	r.RouteCtrl.DelPodCIDRs(cidrs...)
	for _, c := range cidrs {
		if err := r.RouteCtrl.DelRouteByDst(c); err != nil {
			klog.Errorf("Failed to del route for old cidr: %s", c)
			errs = append(errs, err)
		}
	}

	// todo process ovsdpflow

	if len(errs) > 0 {
		return uerr.NewAggregate(errs)
	}
	r.deleteOldCIDRsByKey(k, cidrs...)
	return nil
}

func (r *Reconciler) addIPPoolNewCIDRs(k types.NamespacedName, cidr string) error {
	if cidr == "" {
		return nil
	}

	var errs []error
	r.IptCtrl.InsertPodCIDRs(cidr)
	if err := r.IptCtrl.AddRuleByCIDR(cidr); err != nil {
		klog.Errorf("Failed to add iptables rule for ippool cidr %s: %v", cidr, err)
		errs = append(errs, err)
	}

	r.RouteCtrl.InsertPodCIDRs(cidr)
	if err := r.RouteCtrl.AddRouteByDst(cidr); err != nil {
		klog.Errorf("Failed to add route for ippool cidr %s: %v", cidr, err)
		errs = append(errs, err)
	}

	// todo add ovsdp flows

	if len(errs) > 0 {
		return uerr.NewAggregate(errs)
	}
	return nil
}
