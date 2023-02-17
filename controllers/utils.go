package controller

import (
	"fmt"
	"github.com/wenbingz/k8s-resource-reservation/pkg/config"
	v1 "k8s.io/api/core/v1"
	"strconv"
	"strings"
)

func getReservationAppName(pod *v1.Pod) (string, bool) {
	appName, ok := pod.Labels[config.ReservationAppLabel]
	return appName, ok
}

func createMetaNamespaceKey(namespace string, name string) string {
	return fmt.Sprintf("%s/%s", namespace, name)
}

func splitReservationPodName(podName string) (string, int, int) {
	index := strings.LastIndex(podName, "-")
	replicaId, _ := strconv.Atoi(podName[index:])
	podName = podName[:index]
	index = strings.LastIndex(podName, "-")
	resourceId, _ := strconv.Atoi(podName[index:])
	return podName[:index], resourceId, replicaId
}
