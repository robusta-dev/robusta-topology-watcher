package main

import (
	. "go.uber.org/zap"
	"k8s.io/client-go/tools/cache"
	"k8s.io/client-go/util/workqueue"
)

type eventType int

const (
	ADDED eventType = iota
	UPDATED
	DELETED
)

type InformerEvent struct {
	EventType eventType
	Obj interface{}
	OldObj interface{}
}

// InformerProxy exists because handlers for Kubernetes informers need to be immediate and can't take a lot of time
// to handle individual events. To solve that problem we place an InformerProxy between the informer and the actual
// handler. All that proxy does is store items to handle in a queue.
// We're using a RateLimitingInterface queue but we could have also used a regular queue as we're not taking
// advantage of rate-limiting features or retries at present time.
//
// note: InformerProxy also takes inputs from multiple informers running in multiple goroutines and
// hands off that stream of data to a single goroutine via the queue
type InformerProxy struct {
	logger  *SugaredLogger
	queue   workqueue.RateLimitingInterface
	handler cache.ResourceEventHandler
}

func (w *InformerProxy) OnAdd(obj interface{}) {
	ev := InformerEvent{
		EventType: ADDED,
		Obj: obj,
		OldObj: nil,
	}
	w.queue.Add(ev)
}

func (w *InformerProxy) OnUpdate(oldObj, obj interface{}) {
	ev := InformerEvent{
		EventType: UPDATED,
		Obj: obj,
		OldObj: oldObj,
	}
	w.queue.Add(ev)
}

func (w *InformerProxy) OnDelete(obj interface{}) {
	ev := InformerEvent{
		EventType: DELETED,
		Obj: obj,
		OldObj: nil,
	}
	w.queue.Add(ev)
}

func (w *InformerProxy) Run() {
	for {
		item, quit := w.queue.Get()
		if quit {
			w.logger.Info("quiting informer proxy")
			return
		}

		ev, ok := item.(InformerEvent)
		if !ok {
			w.queue.Done(item)
			w.logger.Error("item in queue has wrong format")
			continue
		}

		switch ev.EventType {
		case ADDED:
			w.handler.OnAdd(ev.Obj)
		case UPDATED:
			w.handler.OnUpdate(ev.Obj, ev.OldObj)
		case DELETED:
			w.handler.OnDelete(ev.Obj)
		default:
			w.logger.Error("eventType is unknown", String("eventType", string(ev.EventType)))
		}
		w.queue.Done(item)
	}
}

func (w *InformerProxy) Stop() {
	w.queue.ShutDown()
}

func NewInformerProxy(logger *SugaredLogger, handler cache.ResourceEventHandler) *InformerProxy {
	return &InformerProxy{
		logger: logger,
		queue: workqueue.NewRateLimitingQueue(workqueue.DefaultControllerRateLimiter()),
		handler: handler,
	}
}