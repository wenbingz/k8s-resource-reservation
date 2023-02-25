package controller

import (
	"fmt"
	"github.com/golang/glog"
	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/tools/cache"
)

type PodEventHandler struct {
	enqueueFunc       func(reserveKey interface{})
	reservationLister cache.GenericLister
}

func newPodEventHandler(enqueueFunc func(interface{}), lister cache.GenericLister) *PodEventHandler {
	return &PodEventHandler{
		enqueueFunc:       enqueueFunc,
		reservationLister: lister,
	}
}

func (handler *PodEventHandler) onPodAdd(obj interface{}) {
	fmt.Println("into on add key")
	var pod *v1.Pod
	runtime.DefaultUnstructuredConverter.FromUnstructured(obj.(*unstructured.Unstructured).Object, &pod)
	fmt.Println(pod.Name + " --- " + pod.Namespace)
	handler.enqueueKeyForReservation(pod)
}

func (handler *PodEventHandler) onPodUpdate(old, updated interface{}) {
	fmt.Println("detect pod update event")
	var oldPod *v1.Pod
	var updatedPod *v1.Pod
	runtime.DefaultUnstructuredConverter.FromUnstructured(old.(*unstructured.Unstructured).Object, &oldPod)
	runtime.DefaultUnstructuredConverter.FromUnstructured(updated.(*unstructured.Unstructured).Object, &updatedPod)
	fmt.Println(oldPod.Namespace, " --------------- ", oldPod.Name)
	fmt.Sprintf("detect update event to pod %s - %s", oldPod.Namespace, oldPod.Name)
	fmt.Println(updatedPod.ResourceVersion, oldPod.ResourceVersion)
	if updatedPod.ResourceVersion == oldPod.ResourceVersion {
		return
	}
	glog.V(2).Infof("Pod %s updated in namespace %s.", updatedPod.GetName(), updatedPod.GetNamespace())
	handler.enqueueKeyForReservation(updatedPod)
}

func (handler *PodEventHandler) onPodDeleted(obj interface{}) {
	objUnstructured, ok := obj.(*unstructured.Unstructured)
	var deletedPod *v1.Pod
	if !ok {
		tomeStone, ok := obj.(cache.DeletedFinalStateUnknown)
		if !ok {
			fmt.Errorf("could not get object from tombstone %+v", obj)
			return
		}
		runtime.DefaultUnstructuredConverter.FromUnstructured(tomeStone.Obj.(*unstructured.Unstructured).Object, &deletedPod)
	} else {
		runtime.DefaultUnstructuredConverter.FromUnstructured(objUnstructured.Object, &deletedPod)
	}
	if deletedPod == nil {
		return
	}
	fmt.Println("detect delete event to pod " + deletedPod.Name + " - " + deletedPod.Namespace)
	handler.enqueueKeyForReservation(deletedPod)
}

func (handler *PodEventHandler) enqueueKeyForReservation(pod *v1.Pod) {
	appName, ok := getReservationAppName(pod)
	fmt.Println("app name ", appName, ok)
	if !ok {
		return
	}
	fmt.Println(handler.reservationLister, "*********************************")
	_, err := handler.reservationLister.ByNamespace(pod.Namespace).Get(appName)
	fmt.Println("err is ", err)
	if err != nil {
		fmt.Println("cannot get the targeted reservation. ", err)
		return
	}
	appKey := createMetaNamespaceKey(pod.Namespace, appName)
	handler.enqueueFunc(appKey)
}
