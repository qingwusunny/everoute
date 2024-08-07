package secret

import (
	"bytes"
	"context"
	"fmt"

	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/klog/v2"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/event"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/source"

	"github.com/everoute/everoute/pkg/constants"
	ersource "github.com/everoute/everoute/pkg/source"
)

type Process struct {
	ERCli    client.Client
	TowerCli client.Client
}

func (p *Process) SetupWithManager(mgr ctrl.Manager, queue chan event.GenericEvent, syncCaches ...ersource.SyncCache) error {
	if mgr == nil {
		klog.Error("Can't setup secret-process controller with nil manager")
		return fmt.Errorf("can't setup secret-process controller with nil manager")
	}
	if queue == nil {
		klog.Error("Param queue is nil")
		return fmt.Errorf("param queue is nil")
	}
	if p.ERCli == nil || p.TowerCli == nil {
		klog.Errorf("Invalid param client, ercli %v, towercli %v", p.ERCli, p.TowerCli)
		return fmt.Errorf("invalid param")
	}
	cm, err := controller.New("secret-process", mgr, controller.Options{
		Reconciler: p,
	})
	if err != nil {
		klog.Errorf("Failed to new secret-process controller: %s", err)
		return err
	}
	err = cm.Watch(&ersource.SyncingChannel{Channel: source.Channel{Source: queue}, SyncCaches: syncCaches}, &handler.EnqueueRequestForObject{})
	if err != nil {
		klog.Errorf("Failed to watch channel for secret-process controller: %s", err)
	}
	return err
}

func (p *Process) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	log := ctrl.LoggerFrom(ctx)
	log.Info("Reconcile start")
	defer log.Info("Reconcile end")

	exp := types.NamespacedName{
		Namespace: constants.TowerSKSKubeconfigNs,
		Name: constants.TowerSKSKubeconfigName,
	}
	if req.NamespacedName != exp {
		log.Error(nil, "Unexpect secret, skip process")
		return ctrl.Result{}, nil
	}
	towerObj := corev1.Secret{}
	if err := p.TowerCli.Get(ctx, req.NamespacedName, &towerObj); err != nil {
		if apierrors.IsNotFound(err) {
			return ctrl.Result{}, p.delete(ctx)
		}
		log.Error(err, "Failed to get sks kubeconfig from tower")
		return ctrl.Result{}, err
	}
	if towerObj.ObjectMeta.DeletionTimestamp != nil {
		return ctrl.Result{}, p.delete(ctx)
	}

	if towerObj.Data["value"] == nil {
		e := fmt.Errorf("sks kubeconfig in tower is invalid")
		log.Error(e, "sks kubeconfig in tower value is nil")
		return ctrl.Result{}, e
	}
	return ctrl.Result{}, p.createOrUpdate(ctx, &towerObj)
}

func (p *Process) delete(ctx context.Context) error {
	log := ctrl.LoggerFrom(ctx)
	erObj := corev1.Secret{}
	key := types.NamespacedName{
		Namespace: constants.SKSKubeconfigNs,
		Name:      constants.SKSKubeconfigName,
	}
	log = log.WithValues("erSecret", key)
	if err := p.ERCli.Get(ctx, key, &erObj); err != nil {
		if apierrors.IsNotFound(err) {
			log.Info("SKS kubeconfig has been deleted in everoute")
			return nil
		}
		log.Error(err, "Failed to get sks kubeconfig in everoute")
		return err
	}

	if err := p.ERCli.Delete(ctx, &erObj); err != nil {
		log.Error(err, "failed to delete sks kubeconfig in everoute")
		return err
	}
	log.Info("Success to delete sks kubeconfig in everoute")
	return nil
}

func (p *Process) createOrUpdate(ctx context.Context, towerObj *corev1.Secret) error {
	log := ctrl.LoggerFrom(ctx)
	key := types.NamespacedName{
		Namespace: constants.SKSKubeconfigNs,
		Name:      constants.SKSKubeconfigName,
	}
	log = log.WithValues("erSecret", key)
	ctrl.LoggerInto(ctx, log)
	erObj := corev1.Secret{}
	if err := p.ERCli.Get(ctx, key, &erObj); err != nil {
		if apierrors.IsNotFound(err) {
			return p.create(ctx, towerObj)
		}
		log.Error(err, "Failed to get sks kubeconfig in everoute")
		return err
	}
	if bytes.Equal(erObj.Data["value"], towerObj.Data["value"]) {
		return nil
	}
	erObj.Data["value"] = bytes.Clone(towerObj.Data["value"])
	if err := p.ERCli.Update(ctx, &erObj); err != nil {
		log.Error(err, "Failed to update sks kubeconfig to everoute")
		return err
	}
	log.Info("Success to update sks kubeconfig to everoute")
	return nil
}

func (p *Process) create(ctx context.Context, towerObj *corev1.Secret) error {
	log := ctrl.LoggerFrom(ctx)
	erObj := corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Namespace:  constants.SKSKubeconfigNs,
			Name:       constants.SKSKubeconfigName,
			Finalizers: []string{constants.SKSKubeconfigFinalizer},
		},
		Data: make(map[string][]byte, 1),
	}
	erObj.Data["value"] = bytes.Clone(towerObj.Data["value"])

	if err := p.ERCli.Create(ctx, &erObj); err != nil {
		log.Error(err, "Failed to create sks kubeconfig to everoute")
		return err
	}
	log.Info("Success create sks kubeconfig to everoute")
	return nil
}
