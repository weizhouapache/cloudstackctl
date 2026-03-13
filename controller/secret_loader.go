package controller

import (
	"context"
	"log"
	"os"
	"strings"

	"github.com/apache/cloudstack-go/v2/cloudstack"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
)

// LoadCloudStackClientFromK8s attempts to read a Kubernetes Secret containing
// CloudStack credentials and returns an initialized CloudStack client.
// Expected Secret keys: api-key, secret-key, api-url, verify-ssl (optional).
// Environment variables to override default secret name/namespace:
// - CLOUDSTACK_SECRET_NAME (default: cloudstack-credentials)
// - CLOUDSTACK_SECRET_NAMESPACE (default: default)
func LoadCloudStackClientFromK8s() (*cloudstack.CloudStackClient, error) {
	secretName := os.Getenv("CLOUDSTACK_SECRET_NAME")
	if secretName == "" {
		secretName = "cloudstack-credentials"
	}
	namespace := os.Getenv("CLOUDSTACK_SECRET_NAMESPACE")
	if namespace == "" {
		namespace = "default"
	}

	// In-cluster config
	cfg, err := rest.InClusterConfig()
	if err != nil {
		return nil, err
	}

	clientset, err := kubernetes.NewForConfig(cfg)
	if err != nil {
		return nil, err
	}

	secret, err := clientset.CoreV1().Secrets(namespace).Get(context.Background(), secretName, metav1.GetOptions{})
	if err != nil {
		return nil, err
	}

	data := secret.Data
	apiKey := strings.TrimSpace(string(data["api-key"]))
	secretKey := strings.TrimSpace(string(data["secret-key"]))
	endpoint := strings.TrimSpace(string(data["api-url"]))
	vs := "true"
	if b, ok := data["verify-ssl"]; ok {
		vs = strings.TrimSpace(string(b))
	}
	verifySSL := true
	if strings.EqualFold(vs, "false") || strings.EqualFold(vs, "0") {
		verifySSL = false
	}

	log.Println("Loaded CloudStack credentials from Kubernetes Secret", secretName, "in namespace", namespace)
	client := cloudstack.NewAsyncClient(endpoint, apiKey, secretKey, verifySSL)
	return client, nil
}
