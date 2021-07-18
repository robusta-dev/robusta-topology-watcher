package main

import (
	. "go.uber.org/zap"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/client-go/tools/cache"
)

type webhookHandler struct {
	logger *Logger
}

func (w *webhookHandler) logEvent(obj interface{}, msg string) {
	u := obj.(*unstructured.Unstructured)
	w.logger.With(
		String("name", u.GetName()),
		String("namespace", u.GetNamespace()),
		String("group-version-kind", u.GroupVersionKind().String())).Info(msg)

	//bytes, err := u.MarshalJSON()
	//if err != nil {
	//	w.logger.With(Error(err)).Warn("error marshalling to json")
	//} else {
	//	w.logger.With(String("bytes", string(bytes))).Info("event details")
	//}
}
func (w *webhookHandler) OnAdd(obj interface{}) {
	w.logEvent(obj, "received add event")
}

func (w *webhookHandler) OnUpdate(oldObj, obj interface{}) {
	w.logEvent(obj, "received update event")
}

func (w *webhookHandler) OnDelete(obj interface{}) {
	w.logEvent(obj, "received delete event")
}

func NewWebhookHandler(logger *SugaredLogger) cache.ResourceEventHandler {
	return &webhookHandler{
		// this module is a hot-path so we desugar the logger to improve performance
		logger.Named("webhook-handler").Desugar(),
	}
}
