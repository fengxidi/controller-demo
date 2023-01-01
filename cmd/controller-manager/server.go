/*
Copyright 2022.

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

package main

import (
	"fmt"
	"time"

	clientset "controller-demo/pkg/client/clientset/versioned"
	informer "controller-demo/pkg/client/informers/externalversions"

	"github.com/spf13/cobra"
	"github.com/spf13/pflag"

	"k8s.io/client-go/kubernetes"
	_ "k8s.io/client-go/plugin/pkg/client/auth"

	"k8s.io/klog/v2"

	"controller-demo/pkg/controller"

	"k8s.io/client-go/tools/clientcmd"
)

var (
	defaultResync time.Duration
	KubeConfig    string
	MasterHost    string
)

func init() {
	defaultResync = time.Second * 30
}

func AddFlag() *pflag.FlagSet {
	fss := pflag.NewFlagSet("config", pflag.ExitOnError)
	fss.StringVar(&KubeConfig, "kubeconfig", "", "Path to a kubeconfig. Only required if out-of-cluster.")
	fss.StringVar(&MasterHost, "master", "", "The address of the Kubernetes API server. Overrides any value in kubeconfig. Only required if out-of-cluster.")

	return fss

}

func NewControllerCommand() *cobra.Command {

	cmd := &cobra.Command{
		Use: "greatdb-operator",
		Long: `Greatdb operator is a project developed based on kubernetes 
		to manage the deployment, maintenance and other functions 
		of greatdb databases on the kubernetes platform`,
		RunE: func(cmd *cobra.Command, args []string) error {
			return run()
		},
		Args: func(cmd *cobra.Command, args []string) error {
			for _, arg := range args {
				if len(arg) > 0 {
					return fmt.Errorf("%q does not take any argument,got %q", cmd.CommandPath(), args)
				}
			}
			return nil
		},
		Example: `
        Print version number
           controller -v or controller --version
		   `,

		Version: "v0.1",
	}
	fs := cmd.Flags()
	fs.AddFlagSet(AddFlag())

	return cmd

}

func run() error {

	stopCh := make(chan struct{})
	// 基于 k8s的配置生成 config
	// 本地调试时，传入对应参数，
	// 打包在k8s环境运行则不需要传递任何参数，
	// 而是采用为pod配置的serviceacount作为认证信息， BuildConfigFromFlags这个函数会自动获取
	// MasterHost： k8s master节点的 ip【kube-apiserver所在节点】
	// KubeConfig： k8s的认证信息， 一般安装完成k8s后,如果拷贝了配置 该配置在 /root/.kube/config，没有拷贝配置，则在 /etc/kubernetes/admin.conf
	cfg, err := clientcmd.BuildConfigFromFlags(MasterHost, KubeConfig)
	if err != nil {
		klog.Error(err.Error())
		return err
	}
	// 基于配置实例 k8s 客户端
	kubeclient, err := kubernetes.NewForConfig(cfg)
	if err != nil {
		klog.Error(err.Error())
		return err
	}
	// 这里就可以直接使用 kubeclient 去获取集群资源的相关信息了【在有对应资源权限的条件下】
	// 例如 ： kubeclient.CoreV1().Pods("default").Get(context.TODO(),"test",metav1.GetOptions{})

	// 统一基于配置得到 blockdevice资源的 client
	Client, err := clientset.NewForConfig(cfg)
	if err != nil {
		klog.Error(err.Error())
		return err
	}
	// 获取到informer实例工厂
	BlockInformerFactory := informer.NewSharedInformerFactory(Client, defaultResync)
	//
	lister := BlockInformerFactory.Suxueit().V1alpha1().BlockDevices().Lister()

	ctrl := controller.NewController(Client, kubeclient, lister, BlockInformerFactory)
	// 开始监听资源，这里进行start后再注册的 informer和 lister 将不会生效，因此我们提前实例化了controller
	BlockInformerFactory.Start(stopCh)
	// start controller
	ctrl.Run(stopCh)

	return nil
}

func main() {
	cmd := NewControllerCommand()
	err := cmd.Execute()

	if err != nil {
		klog.Errorf("command failed %q", err)
	}
}
