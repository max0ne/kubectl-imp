package kube

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

	"github.com/pkg/errors"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/clientcmd"
	clientcmdapi "k8s.io/client-go/tools/clientcmd/api"
	"k8s.io/client-go/util/homedir"

	_ "k8s.io/client-go/plugin/pkg/client/auth/azure"
	_ "k8s.io/client-go/plugin/pkg/client/auth/gcp"
	_ "k8s.io/client-go/plugin/pkg/client/auth/oidc"
	_ "k8s.io/client-go/plugin/pkg/client/auth/openstack"
)

// CreateKubeconfigForServiceAccount creates a kubeconfig object for the given service account
// It uses the currently configured global k8s credential found from process environment
func CreateKubeconfigForServiceAccount(
	namespace, serviceAccount string,
) (*clientcmdapi.Config, error) {
	// Load kubeconfig
	clientConfig, err := loadKubeconfig()
	if err != nil {
		return nil, errors.Wrap(err, "unable to load kubeconfig")
	}
	restConfig, err := clientConfig.ClientConfig()
	if err != nil {
		return nil, errors.Wrap(err, "unable to create k8s client")
	}
	client, err := kubernetes.NewForConfig(restConfig)
	if err != nil {
		return nil, errors.Wrap(err, "unable to create k8s client")
	}

	// Resolve cluster endpoint from client config
	rawConfig, err := clientConfig.RawConfig()
	if err != nil {
		return nil, errors.Wrap(err, "unable to resolve cluster endpoint from kube config")
	}
	currentContext := rawConfig.Contexts[rawConfig.CurrentContext]

	// Resolve namespace in kubeconfig.
	kubeconfigNamespace, _, err := clientConfig.Namespace()
	if err != nil {
		return nil, errors.Wrap(err, "unable to resolve namespace from kubeconfig")
	}
	if namespace == "" {
		namespace = kubeconfigNamespace
	}

	// fetch the secert with token of the service account
	secret, err := fetchServiceAccountSecret(client, namespace, serviceAccount)
	if err != nil {
		return nil, errors.Wrap(err, "unable to get service account secret")
	}

	return createKubeconfig(
		currentContext.Cluster,
		rawConfig.Clusters[currentContext.Cluster].Server,
		kubeconfigNamespace,
		secret.Data["token"],
		secret.Data["ca.crt"],
	), nil
}

// loadKubeconfig tries to load kubeconfig from well known locations
func loadKubeconfig() (clientcmd.ClientConfig, error) {
	var kubeconfigPath string
	kubeconfigPath, ok := os.LookupEnv("KUBECONFIG")
	if !ok {
		if home := homedir.HomeDir(); home != "" {
			kubeconfigPath = filepath.Join(home, ".kube", "config")
		}
	}
	if kubeconfigPath == "" {
		return nil, errors.New("Unable to resolve kubeconfig path")
	}

	apiConfig, err := clientcmd.LoadFromFile(kubeconfigPath)
	if err != nil {
		return nil, fmt.Errorf("Unable to load kubeconfig file at path %s: %s", kubeconfigPath, err)
	}

	return clientcmd.NewDefaultClientConfig(*apiConfig, nil), nil
}

// fetchServiceAccountSecret fetches the secret that contains credentials to the given service account
func fetchServiceAccountSecret(clientset kubernetes.Interface, namespace, serviceAccount string) (*corev1.Secret, error) {
	ctx := context.Background()
	sa, err := clientset.CoreV1().ServiceAccounts(namespace).Get(ctx, serviceAccount, metav1.GetOptions{})
	if err != nil {
		return nil, fmt.Errorf("Unable to get service account %s: %s", serviceAccount, err)
	}

	if len(sa.Secrets) < 1 {
		return nil, fmt.Errorf("Service account %s does not have any secrets", serviceAccount)
	}

	secretName := sa.Secrets[0]
	secret, err := clientset.CoreV1().Secrets(namespace).Get(ctx, secretName.Name, metav1.GetOptions{})
	if err != nil {
		return nil, fmt.Errorf("Unable to get secret %s: %s", secretName, err)
	}

	return secret, nil
}

func createKubeconfig(clusterName, server, namespace string, token, caCert []byte) *clientcmdapi.Config {
	resultConfig := clientcmdapi.NewConfig()
	resultConfig.Clusters[clusterName] = &clientcmdapi.Cluster{
		Server:                   server,
		CertificateAuthorityData: caCert,
	}
	resultConfig.Contexts[clusterName] = &clientcmdapi.Context{
		Cluster:   clusterName,
		AuthInfo:  clusterName,
		Namespace: namespace,
	}
	resultConfig.AuthInfos[clusterName] = &clientcmdapi.AuthInfo{
		Token: string(token),
	}
	resultConfig.CurrentContext = clusterName
	return resultConfig
}
