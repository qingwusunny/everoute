package computecluster

import (
	"context"
	"time"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/sets"
	"k8s.io/apimachinery/pkg/util/wait"
	k8sinformers "k8s.io/client-go/informers"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/cache"
	"k8s.io/client-go/util/workqueue"
	"k8s.io/klog/v2"

	msconst "github.com/everoute/everoute/pkg/constants/ms"
	"github.com/everoute/everoute/plugin/tower/pkg/informer"
	"github.com/everoute/everoute/plugin/tower/pkg/schema"
)

type Controller struct {
	EverouteClusterID  string
	ConfigmapNamespace string

	ctx  context.Context
	name string

	configmapLister         cache.Indexer
	erClusterLister         cache.Indexer
	configmapInformerSynced cache.InformerSynced
	erClusterInformerSynced cache.InformerSynced

	erCli          kubernetes.Interface
	reconcileQueue workqueue.RateLimitingInterface
}

func (c *Controller) Setup(towerFactory informer.SharedInformerFactory, erFactory k8sinformers.SharedInformerFactory, erCli kubernetes.Interface) {
	c.name = "ComputeController"
	c.erCli = erCli
	c.reconcileQueue = workqueue.NewRateLimitingQueue(workqueue.DefaultControllerRateLimiter())

	configMapInformer := erFactory.Core().V1().ConfigMaps().Informer()
	configMapInformer.AddEventHandler(cache.ResourceEventHandlerFuncs{
		AddFunc:    c.handleConfigMap,
		DeleteFunc: c.handleConfigMap,
		UpdateFunc: c.handleConfigMapUpdate,
	})
	c.configmapLister = configMapInformer.GetIndexer()
	c.configmapInformerSynced = configMapInformer.HasSynced

	erClusterInformer := towerFactory.EverouteCluster()
	erClusterInformer.AddEventHandler(cache.ResourceEventHandlerFuncs{
		AddFunc:    c.handleCluster,
		UpdateFunc: c.handleClusterUpdate,
		DeleteFunc: c.handleCluster,
	})
	c.erClusterLister = erClusterInformer.GetIndexer()
	c.erClusterInformerSynced = erClusterInformer.HasSynced
}

func (c *Controller) Run(ctx context.Context) {
	defer c.reconcileQueue.ShutDown()

	if !cache.WaitForNamedCacheSync(c.name, ctx.Done(),
		c.configmapInformerSynced,
		c.erClusterInformerSynced,
	) {
		return
	}
	c.ctx = ctx

	go wait.Until(informer.ReconcileWorker(c.name, c.reconcileQueue, c.reconcile), time.Second, ctx.Done())

	<-ctx.Done()
}

func (c *Controller) handleConfigMap(obj interface{}) {
	unknow, ok := obj.(cache.DeletedFinalStateUnknown)
	if ok {
		obj = unknow.Obj
	}
	elfCfg := obj.(*corev1.ConfigMap)
	if elfCfg.Name == msconst.ELFConfigmapName && elfCfg.Namespace == c.ConfigmapNamespace {
		c.reconcileQueue.Add(c.EverouteClusterID)
	}
}

func (c *Controller) handleConfigMapUpdate(oldObj, newObj interface{}) {
	oldCfg := oldObj.(*corev1.ConfigMap)
	newCfg := newObj.(*corev1.ConfigMap)
	if newCfg.Name != msconst.ELFConfigmapName || newCfg.Namespace != c.ConfigmapNamespace {
		return
	}
	if len(oldCfg.Data) == 0 && len(newCfg.Data) == 0 {
		return
	}
	if len(oldCfg.Data) == 0 || len(newCfg.Data) == 0 {
		c.reconcileQueue.Add(c.EverouteClusterID)
		return
	}
	oldELFs := make(sets.Set[string], len(oldCfg.Data))
	newELFs := make(sets.Set[string], len(newCfg.Data))
	for k := range oldCfg.Data {
		oldELFs.Insert(k)
	}
	for k := range newCfg.Data {
		newELFs.Insert(k)
	}
	if !oldELFs.Equal(newELFs) {
		c.reconcileQueue.Add(c.EverouteClusterID)
	}
}

func (c *Controller) handleCluster(obj interface{}) {
	unknow, ok := obj.(cache.DeletedFinalStateUnknown)
	if ok {
		obj = unknow.Obj
	}
	cluster := obj.(*schema.EverouteCluster)
	if cluster.GetID() != c.EverouteClusterID {
		return
	}
	c.reconcileQueue.Add(c.EverouteClusterID)
}

func (c *Controller) handleClusterUpdate(oldObj, newObj interface{}) {
	oldCluster := oldObj.(*schema.EverouteCluster)
	newCluster := newObj.(*schema.EverouteCluster)

	if newCluster.GetID() != c.EverouteClusterID {
		return
	}

	if newCluster.GetELFs().Equal(oldCluster.GetELFs()) {
		return
	}
	c.reconcileQueue.Add(c.EverouteClusterID)
}

func (c *Controller) reconcile(id string) error {
	klog.Infof("Reconcile start, networkCluster %s", id)
	defer klog.Infof("Reconcile end, networkCluster: %s", id)
	if id != c.EverouteClusterID {
		klog.Warningf("Receive unexpected networkCluster: %s", id)
		return nil
	}

	obj, exists, err := c.configmapLister.GetByKey(c.ConfigmapNamespace + "/" + msconst.ELFConfigmapName)
	if err != nil {
		klog.Errorf("Failed to get configmap store computeClusters: %s", err)
		return err
	}
	if !exists {
		return c.create()
	}
	return c.update(obj.(*corev1.ConfigMap).DeepCopy())
}

func (c *Controller) create() error {
	obj, exists, err := c.erClusterLister.GetByKey(c.EverouteClusterID)
	if err != nil {
		klog.Errorf("Failed to get networkCluster from cloudPlatform: %s", err)
		return err
	}
	configMap := corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: c.ConfigmapNamespace,
			Name:      msconst.ELFConfigmapName,
		},
	}
	if exists {
		configMap.Data = make(map[string]string)
		cluster := obj.(*schema.EverouteCluster)
		for i := range cluster.AgentELFClusters {
			configMap.Data[cluster.AgentELFClusters[i].ID] = ""
		}
	}
	_, err = c.erCli.CoreV1().ConfigMaps(configMap.Namespace).Create(c.ctx, &configMap, metav1.CreateOptions{})
	if err != nil {
		klog.Errorf("Failed to create configmap with computeClusters %v, err: %s", configMap.Data, err)
		return err
	}
	klog.Infof("Success to create configmap with computeClusters %v", configMap.Data)
	return nil
}

func (c *Controller) update(configMap *corev1.ConfigMap) error {
	oldELFs := sets.New[string]()
	if configMap.Data != nil {
		for k := range configMap.Data {
			oldELFs.Insert(k)
		}
	}

	newElfs := sets.New[string]()
	obj, exists, err := c.erClusterLister.GetByKey(c.EverouteClusterID)
	if err != nil {
		klog.Errorf("Failed to get networkCluster in cloudPlatform: %s", err)
		return err
	}
	if exists {
		cluster := obj.(*schema.EverouteCluster)
		newElfs = cluster.GetELFs()
	} else {
		klog.Errorf("Can't found networkCluster %s, skip update configmap which store computeClusters", c.EverouteClusterID)
		return nil
	}

	if oldELFs.Equal(newElfs) {
		return nil
	}

	configMap.Data = make(map[string]string, newElfs.Len())
	for _, elf := range newElfs.UnsortedList() {
		configMap.Data[elf] = ""
	}

	_, err = c.erCli.CoreV1().ConfigMaps(configMap.Namespace).Update(c.ctx, configMap, metav1.UpdateOptions{})
	if err != nil {
		klog.Errorf("Failed to update configmap with computeClusters %v, err: %s", configMap.Data, err)
		return err
	}
	klog.Infof("Success to update configmap with computeClusters %v", configMap.Data)
	return nil
}
