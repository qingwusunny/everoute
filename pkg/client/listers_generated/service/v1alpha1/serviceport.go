/*
Copyright The Everoute Authors.

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

// Code generated by lister-gen. DO NOT EDIT.

package v1alpha1

import (
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/client-go/tools/cache"

	v1alpha1 "github.com/everoute/everoute/pkg/apis/service/v1alpha1"
)

// ServicePortLister helps list ServicePorts.
type ServicePortLister interface {
	// List lists all ServicePorts in the indexer.
	List(selector labels.Selector) (ret []*v1alpha1.ServicePort, err error)
	// ServicePorts returns an object that can list and get ServicePorts.
	ServicePorts(namespace string) ServicePortNamespaceLister
	ServicePortListerExpansion
}

// servicePortLister implements the ServicePortLister interface.
type servicePortLister struct {
	indexer cache.Indexer
}

// NewServicePortLister returns a new ServicePortLister.
func NewServicePortLister(indexer cache.Indexer) ServicePortLister {
	return &servicePortLister{indexer: indexer}
}

// List lists all ServicePorts in the indexer.
func (s *servicePortLister) List(selector labels.Selector) (ret []*v1alpha1.ServicePort, err error) {
	err = cache.ListAll(s.indexer, selector, func(m interface{}) {
		ret = append(ret, m.(*v1alpha1.ServicePort))
	})
	return ret, err
}

// ServicePorts returns an object that can list and get ServicePorts.
func (s *servicePortLister) ServicePorts(namespace string) ServicePortNamespaceLister {
	return servicePortNamespaceLister{indexer: s.indexer, namespace: namespace}
}

// ServicePortNamespaceLister helps list and get ServicePorts.
type ServicePortNamespaceLister interface {
	// List lists all ServicePorts in the indexer for a given namespace.
	List(selector labels.Selector) (ret []*v1alpha1.ServicePort, err error)
	// Get retrieves the ServicePort from the indexer for a given namespace and name.
	Get(name string) (*v1alpha1.ServicePort, error)
	ServicePortNamespaceListerExpansion
}

// servicePortNamespaceLister implements the ServicePortNamespaceLister
// interface.
type servicePortNamespaceLister struct {
	indexer   cache.Indexer
	namespace string
}

// List lists all ServicePorts in the indexer for a given namespace.
func (s servicePortNamespaceLister) List(selector labels.Selector) (ret []*v1alpha1.ServicePort, err error) {
	err = cache.ListAllByNamespace(s.indexer, s.namespace, selector, func(m interface{}) {
		ret = append(ret, m.(*v1alpha1.ServicePort))
	})
	return ret, err
}

// Get retrieves the ServicePort from the indexer for a given namespace and name.
func (s servicePortNamespaceLister) Get(name string) (*v1alpha1.ServicePort, error) {
	obj, exists, err := s.indexer.GetByKey(s.namespace + "/" + name)
	if err != nil {
		return nil, err
	}
	if !exists {
		return nil, errors.NewNotFound(v1alpha1.Resource("serviceport"), name)
	}
	return obj.(*v1alpha1.ServicePort), nil
}
