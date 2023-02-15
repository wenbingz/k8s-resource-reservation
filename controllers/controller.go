package controller

import (
	"fmt"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/dynamic/dynamicinformer"
	"k8s.io/client-go/tools/cache"
	"k8s.io/client-go/tools/clientcmd"
	"os"
	"path/filepath"
)

var (
	SparkApplicationResource = schema.GroupVersionResource{
		Group:    "sparkoperator.k8s.io",
		Version:  "v1beta2",
		Resource: "SparkApplication",
	}
)

type Controller struct {
	dynamicCli  dynamic.Interface
	crdInformer cache.SharedIndexInformer
	podLister   cache.Lister
	stopper     chan struct{}
}

func (*Controller) onAdd(obj interface{}) {
	fmt.Println("added an obj! ")
}

func (*Controller) onUpdate(oldObj interface{}, obj interface{}) {
	fmt.Println("update an obj!")
}

func (*Controller) onDelete(obj interface{}) {
	fmt.Println("delete an obj!")
}

func newController() *Controller {
	userHomePath, err := os.UserHomeDir()
	if err != nil {
		fmt.Println("error when get user home dir")
		os.Exit(-1)
	}
	kubeConfigPath := filepath.Join(userHomePath, ".kube", "config")
	fmt.Println(kubeConfigPath)
	kubeConfig, err := clientcmd.BuildConfigFromFlags("", kubeConfigPath)
	if err != nil {
		fmt.Println("error when constructing kube config")
		os.Exit(-1)
	}

	dynamicCli, err := dynamic.NewForConfig(kubeConfig)
	if err != nil {
		fmt.Println("error when constructing dynamic client")
		fmt.Println(dynamicCli, err)
	}
	controller := &Controller{
		dynamicCli: dynamicCli,
	}
	controller.crdInformer = dynamicinformer.NewDynamicSharedInformerFactory(controller.dynamicCli, 0).ForResource(SparkApplicationResource).Informer()
	controller.crdInformer.AddEventHandler(cache.ResourceEventHandlerFuncs{
		AddFunc:    controller.onAdd,
		DeleteFunc: controller.onDelete,
		UpdateFunc: controller.onUpdate,
	})
	controller.stopper = make(chan struct{})

	return controller
}

func (controller *Controller) Run() {
	//
	go controller.crdInformer.Run(controller.stopper)
	//if !cache.WaitForCacheSync(controller.stopper, controller.crdInformer.HasSynced) {
	//      fmt.Println("error when sync cache")
	//      fmt.Println("hello")
	//      return
	//}

	wait.Until(func() {
		//fmt.Println("waiting")
	}, 1000000, controller.stopper)
}

func main() {
	controller := newController()
	fmt.Println(controller)
	controller.Run()
}
