package metrics

import (
	"context"
	"net/http"

	"github.com/everoute/everoute/pkg/constants"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"k8s.io/klog/v2"
)

type controllerMetric struct {
	reg *prometheus.Registry
	ipM *IPMigrateCount
}

func NewControllerMetric() *controllerMetric {
	return &controllerMetric{
		reg: prometheus.NewRegistry(),
		ipM: NewIPMigrateCount(),
	}
}

func (c *controllerMetric) Init() {
	if err := c.reg.Register(c.ipM.data); err != nil {
		klog.Fatalf("Failed to init controllerMetric %s", err)
	}
}

func (c *controllerMetric) GetIPMigrateCount() *IPMigrateCount {
	return c.ipM
}

func (c *controllerMetric) InstallHandler(registryFunc func(path string, handler http.Handler)) {
	registryFunc(constants.MetricPath, promhttp.HandlerFor(c.reg, promhttp.HandlerOpts{
		ErrorHandling: promhttp.HTTPErrorOnError}))
}

func (c *controllerMetric) Run(ctx context.Context) {
	c.ipM.Run(ctx)
}
