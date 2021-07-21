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
	"time"
)

type Watcher interface {
	AddHandler(handlers cache.ResourceEventHandler, resources[] string) error
	Start()
	Stop()
}

type watcher struct {
	informerProxies []*InformerProxy
	informerFactory dynamicinformer.DynamicSharedInformerFactory
	stopCh          chan struct{}
	logger          *zap.SugaredLogger
}

// if there is an error adding one of the resources then the watcher is left in undefined partially-initialized state
func (w *watcher) AddHandler(handler cache.ResourceEventHandler, resources[] string) error {
	proxy := NewInformerProxy(w.logger, handler)
	w.informerProxies = append(w.informerProxies, proxy)

	for _, resource := range resources {
		groupVersionResource, groupResource := schema.ParseResourceArg(resource)
		if groupVersionResource == nil {
			return fmt.Errorf("could not parse %v; groupResource=%v", resource, groupResource)
		}
		informerLister := w.informerFactory.ForResource(*groupVersionResource)
		if informerLister == nil {
			return fmt.Errorf("could not create infomer/lister for %v %v", resource, groupVersionResource)
		}

		informer := informerLister.Informer()
		informer.AddEventHandler(proxy)
	}
	return nil
}

func (w *watcher) Start() {
	w.informerFactory.Start(w.stopCh)

	// we run the informer proxies without waiting for the cache to sync
	// this way if one of the informers has a problem (e.g. if you're trying to create an informer
	// for a non-existent resource type) then the other informers will still work
	for _, proxy := range w.informerProxies {
		proxy.Run()
	}
	// if the informers don't sync within X time then we have an error
	// don't use w.stopCh here as that is unrelated
	timeoutCh := make(chan struct{})
	go func() {
		timer := time.NewTimer(time.Second * 10)
		defer timer.Stop()
		<- timer.C
		close(timeoutCh)
	}()

	synced := w.informerFactory.WaitForCacheSync(timeoutCh)
	for gvr, success := range synced {
		if !success {
			w.logger.Errorf("could not sync informer for %v", gvr)
		}
	}
}

// note that other goroutines will still run for a short period of time after calling this
func (w *watcher) Stop() {
	close(w.stopCh)
	for _, proxy := range w.informerProxies {
		proxy.Stop()
	}
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
		logger: logger,
	}, nil
}
