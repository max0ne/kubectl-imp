/*
Copyright © 2021 NAME HERE <EMAIL ADDRESS>

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/
package cmd

import (
	"fmt"
	"log"
	"math"
	"os"
	"os/exec"
	"path"
	"strings"

	"github.com/max0ne/kubectl-imp/pkg/kube"
	"github.com/spf13/cobra"
	"k8s.io/client-go/tools/clientcmd"
	clientcmdapi "k8s.io/client-go/tools/clientcmd/api"

	_ "k8s.io/client-go/plugin/pkg/client/auth/azure"
	_ "k8s.io/client-go/plugin/pkg/client/auth/gcp"
	_ "k8s.io/client-go/plugin/pkg/client/auth/oidc"
	_ "k8s.io/client-go/plugin/pkg/client/auth/openstack"
)

var (
	namespace string
)

func Execute() {
	cobra.CheckErr(rootCmd.Execute())
}

func init() {
	rootCmd.Flags().StringVarP(&namespace, "namespace", "n", "", "Namespace of Service Account")
}

// rootCmd represents the base command when called without any subcommands
var rootCmd = &cobra.Command{
	Use:   "kubectl-imp",
	Short: "impersonates a service account",
	Example: strings.Join([]string{
		"kubectl imp r2d2 -- kubectl get pods",
		"kubectl imp c3po -n rebel -- kubectl delete deploy --all",
	}, "\n"),
	Run:  run,
	Args: cobra.RangeArgs(2, math.MaxInt8),
}

func run(cmd *cobra.Command, args []string) {
	serviceAccount := args[0]
	subCommandArgs := args[1:]

	createdKubeconfig, err := kube.CreateKubeconfigForServiceAccount(namespace, serviceAccount)
	if err != nil {
		log.Fatalf("Unable to create kubeconfig: %v", err)
	}

	kubeconfigPath, err := storeKubeconfig(createdKubeconfig)
	if err != nil {
		log.Fatalf("Failed writing kubeconfig to file: %v", err)
	}

	os.Exit(executeCommand(kubeconfigPath, subCommandArgs))
}

func storeKubeconfig(kubeconfig *clientcmdapi.Config) (string, error) {
	kubeconfigDir := path.Join(os.TempDir(), "kubectl-imp", "kubeconfig")
	kubeconfigPath := path.Join(kubeconfigDir, kubeconfig.Contexts[kubeconfig.CurrentContext].Cluster)
	if err := os.MkdirAll(kubeconfigDir, os.ModePerm); err != nil {
		return "", err
	}
	if err := clientcmd.WriteToFile(*kubeconfig, kubeconfigPath); err != nil {
		return "", err
	}
	return kubeconfigPath, nil
}

func executeCommand(kubeconfigPath string, args []string) int {
	// Figure out current shell
	shell, ok := os.LookupEnv("SHELL")
	if !ok {
		shell = "sh"
	}
	command := exec.Command(shell, "-c", strings.Join(args, " "))

	// Extend all env var, there's probably a more elegant way to do this
	for _, env := range os.Environ() {
		if strings.Split(env, "=")[0] != "KUBECONFIG" {
			command.Env = append(command.Env, env)
		}
	}

	// Point kubeconfig env var to the generated kubeconfig file
	command.Env = append(command.Env, fmt.Sprintf("KUBECONFIG=%s", kubeconfigPath))

	command.Stdin = os.Stdin
	command.Stdout = os.Stdout
	command.Stderr = os.Stderr
	command.Run()
	return command.ProcessState.ExitCode()
}
