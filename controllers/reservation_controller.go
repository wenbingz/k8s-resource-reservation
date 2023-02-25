/*
Copyright 2023.

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

package controller

import (
	"context"
	"fmt"
	resourcev1alpha1 "github.com/wenbingz/k8s-resource-reservation/api/v1alpha1"
	"github.com/wenbingz/k8s-resource-reservation/pkg/config"
	"golang.org/x/time/rate"
	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/equality"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/dynamic/dynamicinformer"
	"k8s.io/client-go/tools/cache"
	"k8s.io/client-go/tools/clientcmd"
	"k8s.io/client-go/tools/record"
	"k8s.io/client-go/util/workqueue"
	"os"
	"path/filepath"
	"strconv"
)

const (
	queueTokenRefillRate = 50
	queueTokenBucketSize = 500
)

var (
	keyFunc        = cache.DeletionHandlingMetaNamespaceKeyFunc
	ReservationCRD = schema.GroupVersionResource{
		Group:    "resource.scheduling.org",
		Version:  "v1alpha1",
		Resource: "reservations",
	}
	podResource = schema.GroupVersionResource{
		Version:  "v1",
		Resource: "pods",
	}
)

type ReservationController struct {
	Stopper         chan struct{}
	queue           workqueue.RateLimitingInterface
	dynamicCli      dynamic.Interface
	informerFactory dynamicinformer.DynamicSharedInformerFactory
	crdInformer     cache.SharedIndexInformer
	crdLister       cache.GenericLister
	podLister       cache.GenericLister
	cacheSynced     cache.InformerSynced
	podInformer     cache.SharedIndexInformer
	recorder        record.EventRecorder
	hasSynced       cache.InformerSynced
}

func (rc *ReservationController) onAdd(obj interface{}) {
	var reserve *resourcev1alpha1.Reservation
	runtime.DefaultUnstructuredConverter.FromUnstructured(obj.(*unstructured.Unstructured).Object, &reserve)
	fmt.Printf("added Reservation %s in namespace %s\n", reserve.Name, reserve.Namespace)
	rc.enqueue(reserve)
}

func (rc *ReservationController) onUpdate(oldObj interface{}, obj interface{}) {
	var oldReserve *resourcev1alpha1.Reservation
	var newReserve *resourcev1alpha1.Reservation
	runtime.DefaultUnstructuredConverter.FromUnstructured(oldObj.(*unstructured.Unstructured).Object, &oldReserve)
	runtime.DefaultUnstructuredConverter.FromUnstructured(obj.(*unstructured.Unstructured).Object, &newReserve)
	fmt.Println("detect update event to " + oldReserve.Name + " - " + oldReserve.Namespace)
	if oldReserve.ResourceVersion == newReserve.ResourceVersion {
		return
	}
	if !equality.Semantic.DeepEqual(oldReserve, newReserve) {
	}
	fmt.Sprintf("a reservation %s has been updated ", oldReserve.Name)
}

func (rc *ReservationController) onDelete(obj interface{}) {
	objUnstructured, ok := obj.(*unstructured.Unstructured)
	var deletedReserve *resourcev1alpha1.Reservation
	if !ok {
		tomeStone, ok := obj.(cache.DeletedFinalStateUnknown)
		if !ok {
			fmt.Errorf("could not get object from tombstone %+v", obj)
			return
		}
		runtime.DefaultUnstructuredConverter.FromUnstructured(tomeStone.Obj.(*unstructured.Unstructured).Object, &deletedReserve)
	} else {
		runtime.DefaultUnstructuredConverter.FromUnstructured(objUnstructured.Object, &deletedReserve)
	}
	if deletedReserve == nil {
		return
	}
	fmt.Println("detect delete event to " + deletedReserve.Name + " - " + deletedReserve.Namespace)
	rc.handleReservationDelete(deletedReserve)
}

func (rc *ReservationController) handleReservationDelete(obj interface{}) error {
	var reserve *resourcev1alpha1.Reservation
	switch obj.(type) {
	case *resourcev1alpha1.Reservation:
		{
			reserve = obj.(*resourcev1alpha1.Reservation)
		}
	case cache.DeletedFinalStateUnknown:
		{
			reserve = obj.(cache.DeletedFinalStateUnknown).Obj.(*resourcev1alpha1.Reservation)
		}
	}
	fmt.Println("begin to clean pods for deleted reservation ", reserve.Name, " - ", reserve.Namespace)
	pods, err := rc.getReservationPods(reserve)
	if err != nil {
		fmt.Println("error when get reservation placeholder pods. ", err)
	}
	for _, pod := range pods {
		err := rc.dynamicCli.Resource(podResource).Namespace(pod.Namespace).Delete(context.TODO(), pod.Name, metav1.DeleteOptions{})
		if err != nil {
			fmt.Println(err)
		}
	}
	//for _, request := range reserve.Spec.ResourceRequests {
	//	for i := 0; i < request.Replica; i++ {
	//		podName := rc.getPlaceholderPodName(reserve, i, request.ResourceId)
	//		fmt.Println("try to clean placeholder pod ", podName)
	//		_, err := rc.podLister.ByNamespace(reserve.Namespace).Get(podName)
	//		if err != nil && errors.IsNotFound(err) {
	//			fmt.Sprintf("%s - %s pod not found ", reserve.Namespace, podName)
	//		} else {
	//			fmt.Sprintf("trying to remove pod %s - %s", podName, reserve.Namespace)
	//			err := rc.dynamicCli.Resource(podResource).Namespace(reserve.Namespace).Delete(context.TODO(), podName, metav1.DeleteOptions{})
	//			if err != nil {
	//				fmt.Println(err)
	//			}
	//		}
	//	}
	//}
	return nil
}
func NewReservationController() *ReservationController {
	userHomePath, err := os.UserHomeDir()
	if err != nil {
		fmt.Println("error when get user home dir")
		os.Exit(-1)
	}
	kubeConfigPath := filepath.Join(userHomePath, ".kube", "config")
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
	queue := workqueue.NewNamedRateLimitingQueue(&workqueue.BucketRateLimiter{Limiter: rate.NewLimiter(rate.Limit(queueTokenRefillRate), queueTokenBucketSize)},
		"reservation-controller")
	stopper := make(chan struct{})
	controller := &ReservationController{
		dynamicCli: dynamicCli,
		queue:      queue,
		Stopper:    stopper,
	}
	sharedInformerFactory := dynamicinformer.NewDynamicSharedInformerFactory(controller.dynamicCli, 0)
	controller.informerFactory = sharedInformerFactory
	controller.crdInformer = sharedInformerFactory.ForResource(ReservationCRD).Informer()
	controller.crdLister = sharedInformerFactory.ForResource(ReservationCRD).Lister()
	controller.crdInformer.AddEventHandler(cache.ResourceEventHandlerFuncs{
		DeleteFunc: controller.onDelete,
		AddFunc:    controller.onAdd,
		UpdateFunc: controller.onUpdate,
	})

	podEventHandler := newPodEventHandler(controller.queue.AddRateLimited, controller.crdLister)
	// sharedPodInformer := dynamicinformer.NewDynamicSharedInformerFactory(dynamicCli, 0).ForResource(podResource)
	podInformer := sharedInformerFactory.ForResource(podResource).Informer()
	controller.podLister = sharedInformerFactory.ForResource(podResource).Lister()
	podInformer.AddEventHandler(cache.ResourceEventHandlerFuncs{
		AddFunc:    podEventHandler.onPodAdd,
		DeleteFunc: podEventHandler.onPodDeleted,
		UpdateFunc: podEventHandler.onPodUpdate,
	})
	controller.podInformer = podInformer
	controller.hasSynced = func() bool {
		return controller.crdInformer.HasSynced()
		// return controller.crdInformer.HasSynced() && controller.podInformer.HasSynced()
	}
	go sharedInformerFactory.Start(stopper)
	return controller

}

func (rc *ReservationController) enqueue(obj interface{}) {
	key, err := keyFunc(obj)
	if err != nil {
		fmt.Println("error when get key for %v : %v", obj, err)
	}
	rc.queue.AddRateLimited(key)
}

func (rc *ReservationController) getReservation(namespace string, name string) (*resourcev1alpha1.Reservation, error) {
	fmt.Println(namespace + " ---- " + name)
	//reserve, err := rc.crdLister.ByNamespace("default").Get(name)
	reserve, err := rc.dynamicCli.Resource(ReservationCRD).Namespace(namespace).Get(context.TODO(), name, metav1.GetOptions{})
	if err != nil {
		fmt.Println(err)
		if errors.IsNotFound(err) {
			return nil, nil
		}
		return nil, err
	}
	var res *resourcev1alpha1.Reservation
	runtime.DefaultUnstructuredConverter.FromUnstructured(reserve.Object, &res)
	return res, nil
}

func (rc *ReservationController) getPlaceholderPodName(reservation *resourcev1alpha1.Reservation, replicaId int, resourceId int) string {
	return GenerateRandomName(reservation.Name+"-"+strconv.Itoa(resourceId)+"-"+strconv.Itoa(replicaId)+"-", 5)
}

func (rc *ReservationController) getPlaceholderPod(reservation *resourcev1alpha1.Reservation, replicaId int, resourceId int) (*v1.Pod, error) {
	pods, err := rc.podLister.ByNamespace(reservation.Namespace).List(labels.SelectorFromSet(
		map[string]string{
			config.ReservationAppLabel:   reservation.Name,
			config.ReservationResourceId: strconv.Itoa(resourceId),
			config.ReservationReplicaId:  strconv.Itoa(replicaId),
		},
	))
	if err != nil {
		return nil, err
	}
	if pods == nil || len(pods) <= 0 {
		return nil, fmt.Errorf("no target placeholder found")
	}
	var pod *v1.Pod
	runtime.DefaultUnstructuredConverter.FromUnstructured(pods[0].(*unstructured.Unstructured).Object, &pod)
	return pod, nil
}
func (rc *ReservationController) handleNewlyCreatedReservation(reservation *resourcev1alpha1.Reservation) {
	for _, request := range reservation.Spec.ResourceRequests {
		for i := 0; i < request.Replica; i++ {
			pod, err := rc.getPlaceholderPod(reservation, i, request.ResourceId)
			if err != nil {
				fmt.Sprintf("error when get placeholder pod of app name %s, resource id %d and replica id %d", reservation.Name, request.ResourceId, i)
				fmt.Println(err)
			}
			if 1 == 1 || err != nil && errors.IsNotFound(err) {
				podToCreate := rc.assemblyAPod(reservation, i, request.ResourceId, request.Mem, request.Cpu)
				unstructuredPod, err := runtime.DefaultUnstructuredConverter.ToUnstructured(podToCreate)
				if err != nil {
					fmt.Println("error when convert pod into unstructured data")
				}
				_, err = rc.dynamicCli.Resource(podResource).Namespace(reservation.Namespace).Create(context.TODO(), &unstructured.Unstructured{Object: unstructuredPod}, metav1.CreateOptions{})
				if err != nil {
					fmt.Println(err)
				}
			} else {
				fmt.Printf("already found this pod %s - %s \n", pod.Name, reservation.Namespace)
			}
		}
	}
	reservation.Spec.Status.ReservationStatus = resourcev1alpha1.ReservationStatusInProgress
}
func (rc *ReservationController) syncReservation(key string) error {
	fmt.Println("start to sync reservation key " + key)
	namespace, name, err := cache.SplitMetaNamespaceKey(key)
	fmt.Println(namespace + " ------ " + name)
	if err != nil {
		return fmt.Errorf("failed to get the namespace and name from key %s: %v", key, err)
	}
	reserve, err := rc.getReservation(namespace, name)
	if err != nil {
		fmt.Println(err)
		return err
	}
	if reserve == nil {
		return nil
	}
	reserveCopy := reserve.DeepCopy()
	if reserveCopy.Spec.Status.ReservationStatus == "" {
		reserveCopy.Spec.Status.ReservationStatus = resourcev1alpha1.ReservationStatusCreated
	}
	switch reserveCopy.Spec.Status.ReservationStatus {
	case resourcev1alpha1.ReservationStatusCreated:
		rc.handleNewlyCreatedReservation(reserveCopy)
	case resourcev1alpha1.ReservationStatusInProgress:
		{
			fmt.Println("here to update reservation status")
			rc.updateReservationStatusByCheckingPods(reserveCopy)
		}
	case resourcev1alpha1.ReservationStatusFailed:
		{

		}
	case resourcev1alpha1.ReservationStatusTimeout:
		{

		}
	}
	unstructuredObj, _ := runtime.DefaultUnstructuredConverter.ToUnstructured(reserveCopy)
	_, err = rc.dynamicCli.Resource(ReservationCRD).Namespace(reserve.Namespace).Update(context.TODO(), &unstructured.Unstructured{Object: unstructuredObj}, metav1.UpdateOptions{})
	if err != nil {
		fmt.Println(err)
	}
	return nil
}

func (rc *ReservationController) updateReservationStatusByCheckingPods(reservation *resourcev1alpha1.Reservation) {
	pods, err := rc.getReservationPods(reservation)
	if err != nil {
		fmt.Printf("error when getting reservation pods with %s \n", reservation.Name)
		return
	}
	var total int = 0
	for _, request := range reservation.Spec.ResourceRequests {
		total += request.Replica
	}
	var cnt int = 0
	stats := make(map[int]map[int]bool)
	for _, pod := range pods {
		_, resourceId, replicaId, ok := splitReservationPodInfo(pod)
		if !ok {
			fmt.Errorf("error when split pod info ")
			return
		}
		if reservation.Spec.Placeholders == nil {
			reservation.Spec.Placeholders = make(map[string]resourcev1alpha1.PodStatus)
		}
		reservation.Spec.Placeholders[pod.Name] = resourcev1alpha1.PodStatus{
			PodStatus: string(pod.Status.Phase),
		}
		if pod.Status.Phase == v1.PodRunning {
			cnt += 1
		}
		if stats[resourceId] == nil {
			stats[resourceId] = make(map[int]bool)
		}
		stats[resourceId][replicaId] = true
	}
	for _, request := range reservation.Spec.ResourceRequests {
		for r := 0; r < request.Replica; r++ {
			if !stats[request.ResourceId][r] {
				podToCreate := rc.assemblyAPod(reservation, request.ResourceId, r, request.Mem, request.Cpu)
				unstructuredPod, err := runtime.DefaultUnstructuredConverter.ToUnstructured(podToCreate)
				if err != nil {
					fmt.Println("error when convert pod into unstructured data")
				}
				rc.dynamicCli.Resource(podResource).Create(context.TODO(), &unstructured.Unstructured{Object: unstructuredPod}, metav1.CreateOptions{})
			}
		}
	}
	if cnt == total {
		reservation.Spec.Status.ReservationStatus = resourcev1alpha1.ReservationStatusCompleted
	}
}
func (rc *ReservationController) processNextItem() bool {
	fmt.Println("trying to get a key")
	key, quit := rc.queue.Get()
	fmt.Println("get a key ", key)
	if quit {
		return false
	}
	defer rc.queue.Done(key)
	fmt.Println("get a key " + key.(string))
	err := rc.syncReservation(key.(string))
	if err == nil {
		rc.queue.Forget(key)
		return true
	}
	return true
}

func (rc *ReservationController) assemblyAPod(reservation *resourcev1alpha1.Reservation, replicaId int, resourceId int, mem string, core string) *v1.Pod {
	//labels := reservation.Labels
	pod := &v1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      rc.getPlaceholderPodName(reservation, replicaId, resourceId),
			Namespace: reservation.Namespace,
			Labels: map[string]string{
				config.ReservationAppLabel:   reservation.Name,
				config.ReservationReplicaId:  strconv.Itoa(replicaId),
				config.ReservationResourceId: strconv.Itoa(resourceId),
			},
		},
		Spec: v1.PodSpec{
			Containers: []v1.Container{
				{
					Name:            "pause",
					Image:           "docker.io/google/pause:latest",
					ImagePullPolicy: v1.PullIfNotPresent,
					Resources: v1.ResourceRequirements{
						Requests: v1.ResourceList{
							"memory": resource.MustParse(mem),
							"cpu":    resource.MustParse(core),
						},
						Limits: v1.ResourceList{
							"memory": resource.MustParse(mem),
							"cpu":    resource.MustParse(core),
						},
					},
				},
			},
			Tolerations: []v1.Toleration{
				{
					Key:      "node-role.kubernetes.io/master",
					Operator: v1.TolerationOpExists,
					Effect:   v1.TaintEffectNoSchedule,
				},
			},
			PriorityClassName: config.PlaceholderPodPriorityClass,
		},
	}
	return pod
}

func (rc *ReservationController) getReservationPods(reservation *resourcev1alpha1.Reservation) ([]*v1.Pod, error) {
	matchLabels := map[string]string{config.ReservationAppLabel: reservation.Name}
	selector := labels.SelectorFromSet(matchLabels)
	pods, err := rc.podLister.ByNamespace(reservation.Namespace).List(selector)
	if err != nil {
		fmt.Println("fail to get reservation placeholder pods", err)
	}
	var res []*v1.Pod
	for _, p := range pods {
		var temp *v1.Pod
		runtime.DefaultUnstructuredConverter.FromUnstructured(p.(*unstructured.Unstructured).Object, &temp)
		res = append(res, temp)
	}
	return res, err
}

func (rc *ReservationController) RunController() {
	fmt.Println("Launching reservation controller")
	fmt.Println(rc.crdInformer, "-----------", rc.podInformer)
	res := rc.informerFactory.WaitForCacheSync(rc.Stopper)
	if !(res[podResource] && res[ReservationCRD]) {
		fmt.Println("error when sync cache")
		return
	}
	defer utilruntime.HandleCrash()
	for rc.processNextItem() {
		fmt.Println("next loop")
	}
}
