package controller

import (
	"github.com/golang/glog"
	v1 "k8s.io/api/core/v1"
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
	pod := obj.(*v1.Pod)
	handler.enqueueKeyForReservation(pod)
}

func (handler *PodEventHandler) onPodUpdate(old, updated interface{}) {
	oldPod := old.(*v1.Pod)
	updatedPod := updated.(*v1.Pod)

	if updatedPod.ResourceVersion == oldPod.ResourceVersion {
		return
	}
	glog.V(2).Infof("Pod %s updated in namespace %s.", updatedPod.GetName(), updatedPod.GetNamespace())
	handler.enqueueKeyForReservation(updatedPod)
}

func (handler *PodEventHandler) onPodDeleted(obj interface{}) {
	var deletedPod *v1.Pod
	switch obj.(type) {
	case *v1.Pod:
		deletedPod = obj.(*v1.Pod)
	case cache.DeletedFinalStateUnknown:
		deletedPod = obj.(cache.DeletedFinalStateUnknown).Obj.(*v1.Pod)
	}
	if deletedPod == nil {
		return
	}
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
