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
	var oldPod *v1.Pod
	var updatedPod *v1.Pod
	runtime.DefaultUnstructuredConverter.FromUnstructured(old.(*unstructured.Unstructured).Object, &oldPod)
	runtime.DefaultUnstructuredConverter.FromUnstructured(updated.(*unstructured.Unstructured).Object, &updatedPod)

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
	if !ok {
		return
	}
	_, err := handler.reservationLister.ByNamespace(pod.Namespace).Get(appName)
	if err != nil {
		return
	}
	appKey := createMetaNamespaceKey(pod.Namespace, appName)
	handler.enqueueFunc(appKey)
}
