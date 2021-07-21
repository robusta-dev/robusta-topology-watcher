package main

import (
	"fmt"
	. "go.uber.org/zap"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/tools/cache"
	"time"
)

// important: the objects in the cache are READ ONLY as they're shared
// with other watchers subscribed to the same shared informer
type cacheHandler struct {
	logger *Logger
	cache  cache.Store
	stopCh chan interface{}
}

type CacheHandler interface {
	cache.ResourceEventHandler
	Start()
	Stop()
	GetOwner()
}

// TODO: add unit tests for some of these
func ownerReferencesSliceToMap (references []metav1.OwnerReference) map[string]metav1.OwnerReference {
	result := make(map [string]metav1.OwnerReference)
	for _, ref := range references {
		key := ref.APIVersion + "/" + ref.Kind + "/" + ref.Name
		result[key] = ref
	}
	return result
}

// returns items in `a` but not in `b`
func getReferencesDiff (a, b map[string]metav1.OwnerReference) []metav1.OwnerReference {
	diff := make([]metav1.OwnerReference, 0)
	for key, ref := range a {
		if _, ok := b[key]; !ok {
			diff = append(diff, ref)
		}
	}
	return diff
}

func (w *cacheHandler) compareOwnerReferences (old, new *unstructured.Unstructured) {
	oldReferences := ownerReferencesSliceToMap(old.GetOwnerReferences())
	newReferences := ownerReferencesSliceToMap(new.GetOwnerReferences())

	if len(oldReferences) != 0 || len (newReferences) != 0 {
		w.logger.Sugar().Infof("references are old=%v new=%v", oldReferences, newReferences)
	}
	removedReferences := getReferencesDiff(oldReferences, newReferences)
	addedReferences := getReferencesDiff(newReferences, oldReferences)

	if len(removedReferences) != 0 {
		w.logger.Sugar().Warnf("there are removed references: %v", removedReferences)
	}
	if len(addedReferences) != 0 {
		w.logger.Sugar().Warnf("there are added references: %v", addedReferences)
	}
}

func (w *cacheHandler) checkOwnerReferences(newUn *unstructured.Unstructured) {
	oldObj, exists, err := w.cache.Get(newUn)
	if err != nil {
		w.logger.Error("error getting last cached object", Error(err))
		return
	}

	if !exists {
		return
	}

	oldUn, ok := oldObj.(*unstructured.Unstructured)
	if !ok {
		w.logger.Error("cannot cast lastCachedObj to an *unstructured.Unstructured")
		return
	}

	w.compareOwnerReferences(oldUn, newUn)
}

func (w *cacheHandler) updateCache(obj interface{}) {
	un, ok := obj.(*unstructured.Unstructured)
	if !ok {
		w.logger.Error("object is not an *unstructured.Unstructured")
		return
	}

	if len(un.GetOwnerReferences()) > 1 {
		w.logger.Sugar().Warnf("object has more than one owner reference %v", un.GetOwnerReferences())
	}
	w.checkOwnerReferences(un)

	err := w.cache.Add(obj)
	if err != nil {
		w.logger.Error("error adding item to ttl cache", Error(err))
	}
}

func (w *cacheHandler) OnAdd(obj interface{}) {
	w.updateCache(obj)
}

func (w *cacheHandler) OnUpdate(oldObj, obj interface{}) {
	w.updateCache(obj)
}

func (w *cacheHandler) OnDelete(obj interface{}) {
	if _, ok := obj.(cache.DeletedFinalStateUnknown); ok {
		w.logger.Warn("ignoring DeletedFinalStateUnknown in cacheHandler")
		return
	}
	w.updateCache(obj)
}

func (w *cacheHandler) Start() {
	go func() {
		ticker := time.NewTicker(time.Minute)
		for {
			select {
			case <- w.stopCh:
				return
			case <- ticker.C:
				// items are only purged from ttl cache when read, so we do occasional lists to ensure
				// all old items are purged
				w.cache.List()
			}
		}
	}()
}

func (w *cacheHandler) Stop() {
	close(w.stopCh)
}

func (w *cacheHandler) GetOwner() {
	// TODO: climb up the ownership tree while warning if there is more than one owner
}

// GenericKeyFunc is a key func that encodes the objects gvk as part of it's key
func GenericKeyFunc(obj interface{}) (string, error) {
	if d, ok := obj.(cache.DeletedFinalStateUnknown); ok {
		return d.Key, nil
	}

	un, ok := obj.(*unstructured.Unstructured)
	if !ok {
		return "", fmt.Errorf("object is not an Unstructed")
	}

	gvk := un.GroupVersionKind()
	return GetKey(gvk, un.GetName(), un.GetNamespace()), nil
}

func GetKey(gvk schema.GroupVersionKind, name, namespace string) string {
	return gvk.Group + "/" + gvk.Version + "/" + gvk.Kind + "/" + namespace + "/" + name
}

func NewCacheHandler(logger *SugaredLogger) CacheHandler {
	return &cacheHandler{
		// this module is a hot-path so we desugar the logger to improve performance
		logger.Named("cache-handler").Desugar(),
		cache.NewTTLStore(GenericKeyFunc, time.Minute * 30),
		make(chan interface{}),
	}
}
