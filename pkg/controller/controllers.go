package controller

import (
	"controller-demo/pkg/client/clientset/versioned"
	Informer "controller-demo/pkg/client/informers/externalversions"
	lister "controller-demo/pkg/client/listers/device/v1alpha1"
	"controller-demo/pkg/controller/blockdevice"

	"k8s.io/client-go/kubernetes"
)

type controller struct {
	Client        versioned.Interface
	kubeClient    kubernetes.Interface
	Listers       lister.BlockDeviceLister
	blockinformer Informer.SharedInformerFactory
}

func NewController(
	Client versioned.Interface,
	kubeClient kubernetes.Interface,
	Listers lister.BlockDeviceLister,
	blockinformer Informer.SharedInformerFactory) *controller {

	return &controller{
		Client:        Client,
		kubeClient:    kubeClient,
		Listers:       Listers,
		blockinformer: blockinformer,
	}

}

func (ctrl controller) Run(stopCh <-chan struct{}) {
	blockController := blockdevice.NewBlockDeviceController(ctrl.Client, ctrl.kubeClient, ctrl.Listers, ctrl.blockinformer)
	go blockController.Run(1, stopCh)

	<-stopCh
}
