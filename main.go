package main

import (
	"fmt"
	. "go.uber.org/zap"
	"go.uber.org/zap/zapcore"
	"os"
	"os/signal"

	"github.com/mitchellh/go-homedir"
	// see https://www.fullstory.com/blog/connect-to-google-kubernetes-with-gcp-credentials-and-pure-golang
	_ "k8s.io/client-go/plugin/pkg/client/auth/gcp" // register GCP auth provider
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
)

func createKubernetesConfig() (*rest.Config, error) {
	config, err := rest.InClusterConfig()
	if err == nil {
		return config, nil
	}

	kubeConfigPath := os.Getenv("KUBECONFIG")
	if kubeConfigPath == "" {
		kubeConfigPath, err = homedir.Expand("~/.kube/config")
		if err != nil {
			return nil, err
		}
	}

	config, err = clientcmd.BuildConfigFromFlags("", kubeConfigPath)
	return config, err
}

func createRootLogger() (*SugaredLogger, error) {
	config := NewDevelopmentConfig()
	config.EncoderConfig.EncodeLevel = zapcore.CapitalColorLevelEncoder
	logger, err := config.Build()
	if err != nil {
		return nil, err
	}
	return logger.Sugar(), nil
}

func main() {
	logger, err := createRootLogger()
	if err != nil {
		fmt.Printf("error creating logger! %v", err)
		return
	}
	defer func(logger *SugaredLogger) {
		// ignore errors - see https://github.com/uber-go/zap/issues/880
		_ = logger.Sync()
	}(logger)

	cfg, err := createKubernetesConfig()
	if err != nil {
		logger.Fatal("could not create kubernetes config", Error(err))
	}

	watcher, err := NewWatcher(cfg, logger)
	if err != nil {
		logger.Fatal("could not create watcher", Error(err))
	}

	// TODO: fix url
	//webhook := NewWebhookHandler(logger, "http://127.0.0.1")
	//err = watcher.AddHandler(webhook, []string{
	//	"deployments.v1.apps",
	//	"events.v1.events.k8s.io",
	//})
	//if err != nil {
	//	logger.Fatal("could not add webhook listener", Error(err))
	//}
	//

	cache := NewCacheHandler(logger)
	cache.Start()
	defer cache.Stop()

	err = watcher.AddHandler(cache, []string{
		"deployments.v1.apps",
		"events.v1.events.k8s.io",
		"pods.v1.", // we need the trailing dot to signify that this is in the "" group
	})
	if err != nil {
		logger.Fatal("could not add cache listener", Error(err))
	}

	watcher.Start()

	sigCh := make(chan os.Signal, 0)
	signal.Notify(sigCh, os.Kill, os.Interrupt)
	<- sigCh
	watcher.Stop()
}
