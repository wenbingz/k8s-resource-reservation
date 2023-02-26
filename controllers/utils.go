package controller

import (
	"fmt"
	"github.com/wenbingz/k8s-resource-reservation/pkg/config"
	v1 "k8s.io/api/core/v1"
	utilrand "k8s.io/apimachinery/pkg/util/rand"
	"strconv"
)

func getReservationAppName(pod *v1.Pod) (string, bool) {
	fmt.Println(pod.Labels, " ------------- ", config.ReservationAppLabel)
	appName, ok := pod.Labels[config.ReservationAppLabel]
	return appName, ok
}

func createMetaNamespaceKey(namespace string, name string) string {
	return fmt.Sprintf("%s/%s", namespace, name)
}

//func splitReservationPodName(podName string) (string, int, int) {
//	index := strings.LastIndex(podName, "-")
//	replicaId, _ := strconv.Atoi(podName[index:])
//	podName = podName[:index]
//	index = strings.LastIndex(podName, "-")
//	resourceId, _ := strconv.Atoi(podName[index:])
//	return podName[:index], resourceId, replicaId
//}

func SplitReservationPodInfo(pod *v1.Pod) (string, int, int, bool) {
	label := pod.Labels
	appName, ok := label[config.ReservationAppLabel]
	if !ok {
		return "", -1, -1, false
	}
	resourceId, ok := label[config.ReservationResourceId]
	if !ok {
		return "", -1, -1, false
	}
	replicaId, ok := label[config.ReservationReplicaId]
	if !ok {
		return "", -1, -1, false
	}
	resourceIdInt, err := strconv.Atoi(resourceId)
	if err != nil {
		return "", -1, -1, false
	}
	replicaIdInt, err := strconv.Atoi(replicaId)
	if err != nil {
		return "", -1, -1, false
	}
	return appName, resourceIdInt, replicaIdInt, true
}

func GenerateRandomName(base string, randomLen int) string {
	return fmt.Sprintf("%s%s", base, utilrand.String(randomLen))
}
