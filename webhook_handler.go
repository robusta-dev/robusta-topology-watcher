package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	. "go.uber.org/zap"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/tools/cache"
	"net/http"
	"time"
)

type CloudEventMessage struct {
	SpecVersion     string                `json:"specversion"`
	Type            string                `json:"type"`
	Source          string                `json:"source"`
	Subject         string                `json:"subject"`
	ID              string                `json:"id"`
	Time            time.Time             `json:"time"`
	DataContentType string                `json:"datacontenttype"`
	Data            CloudEventMessageData `json:"data"`
}

type CloudEventMessageData struct {
	Operation   string         `json:"operation"`
	Kind        string         `json:"kind"`
	ClusterUid  string         `json:"clusterUid"`
	// TODO: remove description from here and create the description in the runner?
	Description string         `json:"description"`
	ApiVersion  string         `json:"apiVersion"`
	Obj         runtime.Object `json:"obj"`
	OldObj      runtime.Object `json:"oldObj"`
}

type webhookHandler struct {
	url       string
	startTime uint64
	counter   uint64
	logger    *Logger
}

func (w *webhookHandler) prepareMessage(obj, oldObj *unstructured.Unstructured, eventType string) *CloudEventMessage {
	return &CloudEventMessage{
		SpecVersion:     "1.0",
		Type:            "KUBERNETES_TOPOLOGY_CHANGE",
		Source:          "https://github.com/aantn/kubewatch",
		ID:              fmt.Sprintf("%v-%v", w.startTime, w.counter),
		Time:            time.Now(), // note that this is the time of sending not time of event
		DataContentType: "application/json",
		Data: CloudEventMessageData{
			Operation:   eventType,
			// TODO: verify these are the same as the old version...
			Kind:        obj.GetKind(),
			ApiVersion:  obj.GetAPIVersion(),
			ClusterUid:  "TODO",
			Description: "TODO",
			Obj:         obj,
			OldObj:      oldObj,
		},
	}
}

func (w *webhookHandler) postMessage(webhookMessage *CloudEventMessage) error {
	message, err := json.Marshal(webhookMessage)
	if err != nil {
		return err
	}

	req, err := http.NewRequest("POST", w.url, bytes.NewBuffer(message))
	if err != nil {
		return err
	}
	req.Header.Add("Content-Type", "application/json")

	client := &http.Client{}
	_, err = client.Do(req)
	if err != nil {
		return err
	}

	return nil
}
func (w *webhookHandler) logEvent(obj, oldObj interface{}, eventType string) {
	u, ok := obj.(*unstructured.Unstructured)
	if !ok {
		w.logger.Error("obj is not an *unstructured.Unstructured")
		return
	}

	oldU, ok := oldObj.(*unstructured.Unstructured)
	if !ok && oldObj != nil {
		w.logger.Error("oldObj is non-nil but it is not an *unstructured.Unstructured")
		return
	}

	w.logger.Info(eventType,
		String("name", u.GetName()),
		String("namespace", u.GetNamespace()),
		String("group-version-kind", u.GroupVersionKind().String()))

	//bytes, err := u.MarshalJSON()
	//if err != nil {
	//	w.logger.With(Error(err)).Warn("error marshalling to json")
	//} else {
	//	w.logger.With(String("bytes", string(bytes))).Info("event details")
	//}
	w.counter++
	message := w.prepareMessage(u, oldU, eventType)

	err := w.postMessage(message)
	if err != nil {
		w.logger.Error("error posting message", Error(err))
		return
	}

	w.logger.Info("message sent successfully", String("destination", w.url))
}

// TODO: add it to a client-go/util/workqueue - SharedInformer handlers aren't allowed to take a lot of
// time to process items
func (w *webhookHandler) OnAdd(obj interface{}) {
	w.logEvent(obj, nil, "create")
}

func (w *webhookHandler) OnUpdate(oldObj, obj interface{}) {
	w.logEvent(obj, oldObj, "update")
}

func (w *webhookHandler) OnDelete(obj interface{}) {
	// this is OK and can happen if the informer knows the object was deleted but doesn't
	// know its finally state. just send the event through
	if finalStateUnknown, ok := obj.(cache.DeletedFinalStateUnknown); ok {
		w.logger.Warn("got DeletedFinalStateUnknown in webhookHandler")
		w.logEvent(finalStateUnknown.Obj, nil, "delete")
		return
	}
	w.logEvent(obj, nil, "delete")
}

func NewWebhookHandler(logger *SugaredLogger, url string) cache.ResourceEventHandler {
	return &webhookHandler{
		// this module is a hot-path so we desugar the logger to improve performance
		url:       url,
		startTime: uint64(time.Now().Unix()),
		counter: 0,
		logger:    logger.Named("webhook-handler").Desugar(),
	}
}
