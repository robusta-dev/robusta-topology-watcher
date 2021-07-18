package main

import (
	"fmt"
	"go.uber.org/zap"
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

func createRootLogger() (*zap.SugaredLogger, error) {
	config := zap.NewDevelopmentConfig()
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
	defer func(logger *zap.SugaredLogger) {
		err := logger.Sync()
		if err != nil {
			fmt.Printf("error calling sync on logger! %v", err)
		}
	}(logger)

	cfg, err := createKubernetesConfig()
	if err != nil {
		logger.With("error", err).Fatal("could not create kubernetes config")
	}

	watcher, err := NewWatcher(cfg, logger)
	if err != nil {
		logger.With("error", err).Fatal("could not create watcher")
	}

	webhook := NewWebhookHandler(logger)
	// TODO: this isn't ideal... when we have an error listing a specific type we find out only later
	err = watcher.AddHandler(webhook, []string{
		"deployments.v1.apps",
		"events.v1.events.k8s.io",
	})
	if err != nil {
		logger.With("error", err).Fatal("could not add webhook listener")
	}
	watcher.Start()

	sigCh := make(chan os.Signal, 0)
	signal.Notify(sigCh, os.Kill, os.Interrupt)
	<- sigCh

	watcher.Stop()
}
