package main

import (
	"fmt"
	"go.uber.org/zap"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/dynamic/dynamicinformer"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/cache"
)

type Watcher interface {
	AddHandler(handlers cache.ResourceEventHandler, resources[] string) error
	Start()
	Stop()
}

type watcher struct {
	informerFactory dynamicinformer.DynamicSharedInformerFactory
	stopCh 			chan struct{}
}

// if there is an error adding one of the resources then the watcher is left in undefined partially-initialized state
func (w *watcher) AddHandler(handlers cache.ResourceEventHandler, resources[] string) error {
	for _, resource := range resources {
		groupVersionResource, _ := schema.ParseResourceArg(resource)
		if groupVersionResource == nil {
			return fmt.Errorf("could not parse %v", resource)
		}
		informerLister := w.informerFactory.ForResource(*groupVersionResource)
		if informerLister == nil {
			return fmt.Errorf("could not create infomer/lister for %v %v", resource, groupVersionResource)
		}

		informer := informerLister.Informer()
		informer.AddEventHandler(handlers)
	}
	return nil
}

func (w *watcher) Start() {
	w.informerFactory.Start(w.stopCh)
}

// note that other goroutines will still run for a short period of time after calling this
func (w *watcher) Stop() {
	close(w.stopCh)
}

func NewWatcher(restConfig *rest.Config, logger *zap.SugaredLogger) (Watcher, error) {
	if restConfig == nil {
		return nil, fmt.Errorf("createKubernetesConfig must be non-nil")
	}

	client, err := dynamic.NewForConfig(restConfig)
	if err != nil {
		return nil, fmt.Errorf("cannot create dyanmic client: %w", err)
	}

	informerFactory := dynamicinformer.NewFilteredDynamicSharedInformerFactory(client,
		0, v1.NamespaceAll, nil)

	return &watcher{
		informerFactory: informerFactory,
		stopCh: make(chan struct{}),
	}, nil
}
