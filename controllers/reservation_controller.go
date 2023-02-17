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
		Resource: "Reservation",
	}
	podResource = schema.GroupVersionResource{
		Version:  "v1",
		Resource: "pods",
	}
)

type ReservationController struct {
	Stopper     chan struct{}
	queue       workqueue.RateLimitingInterface
	dynamicCli  dynamic.Interface
	crdInformer cache.SharedIndexInformer
	crdLister   cache.GenericLister
	podLister   cache.GenericLister
	cacheSynced cache.InformerSynced
	podInformer cache.SharedIndexInformer
	recorder    record.EventRecorder
}

func (rc *ReservationController) onAdd(obj interface{}) {
	reserve := obj.(*resourcev1alpha1.Reservation)
	fmt.Printf("added Reservation %s in namespace %s\n", reserve.Name, reserve.Namespace)
	rc.enqueue(reserve)
}

func (rc *ReservationController) onUpdate(oldObj interface{}, obj interface{}) {
	oldReserve := oldObj.(*resourcev1alpha1.Reservation)
	newReserve := obj.(*resourcev1alpha1.Reservation)
	if oldReserve.ResourceVersion == newReserve.ResourceVersion {
		return
	}
	if !equality.Semantic.DeepEqual(oldReserve, newReserve) {
	}
	fmt.Sprintf("a reservation %s has been updated ", oldReserve.Name)
}

func (rc *ReservationController) onDelete(obj interface{}) {
	rc.handleReservationDelete(obj)
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
	for _, request := range reserve.Spec.ResourceRequests {
		for i := 0; i < request.Replica; i++ {
			podName := rc.getPlaceholderPodName(reserve, request.ResourceId, i)
			_, err := rc.podLister.ByNamespace(reserve.Namespace).Get(podName)
			if err != nil && errors.IsNotFound(err) {
				fmt.Sprintf("trying to remove pod %s - %s", podName, reserve.Namespace)
				rc.dynamicCli.Resource(podResource).Namespace(reserve.Namespace).Delete(context.TODO(), podName, metav1.DeleteOptions{})
			}
		}
	}
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
	controller.crdInformer = dynamicinformer.NewDynamicSharedInformerFactory(controller.dynamicCli, 0).ForResource(ReservationCRD).Informer()
	controller.crdLister = dynamicinformer.NewDynamicSharedInformerFactory(controller.dynamicCli, 0).ForResource(ReservationCRD).Lister()
	controller.crdInformer.AddEventHandler(cache.ResourceEventHandlerFuncs{
		DeleteFunc: controller.onDelete,
		AddFunc:    controller.onAdd,
		UpdateFunc: controller.onUpdate,
	})

	podEventHandler := newPodEventHandler(controller.queue.AddRateLimited, controller.crdLister)
	podInformer := dynamicinformer.NewDynamicSharedInformerFactory(dynamicCli, 0).ForResource(schema.GroupVersionResource{
		Version:  "v1",
		Resource: "pods",
	}).Informer()
	controller.podLister = dynamicinformer.NewDynamicSharedInformerFactory(dynamicCli, 0).ForResource(podResource).Lister()
	podInformer.AddEventHandler(cache.ResourceEventHandlerFuncs{
		AddFunc:    podEventHandler.onPodAdd,
		DeleteFunc: podEventHandler.onPodDeleted,
		UpdateFunc: podEventHandler.onPodUpdate,
	})
	go controller.crdInformer.Run(stopper)
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
	reserve, err := rc.crdLister.ByNamespace(namespace).Get(name)
	if err != nil {
		if errors.IsNotFound(err) {
			return nil, nil
		}
		return nil, err
	}
	return reserve.(*resourcev1alpha1.Reservation), nil
}

func (rc *ReservationController) getPlaceholderPodName(reservation *resourcev1alpha1.Reservation, replicaId int, resourceId int) string {
	return reservation.Name + "-" + strconv.Itoa(resourceId) + "-" + strconv.Itoa(replicaId)
}

func (rc *ReservationController) handleNewlyCreatedReservation(reservation *resourcev1alpha1.Reservation) {
	for _, request := range reservation.Spec.ResourceRequests {
		for i := 0; i < request.Replica; i++ {
			podName := rc.getPlaceholderPodName(reservation, i, request.ResourceId)
			_, err := rc.podLister.ByNamespace(reservation.Namespace).Get(podName)
			if err != nil && errors.IsNotFound(err) {
				podToCreate := rc.assemblyAPod(reservation, i, request.ResourceId, request.Mem, request.Cpu)
				unstructuredPod, err := runtime.DefaultUnstructuredConverter.ToUnstructured(podToCreate)
				if err != nil {
					fmt.Println("error when convert pod into unstructured data")
				}
				rc.dynamicCli.Resource(podResource).Create(context.TODO(), &unstructured.Unstructured{Object: unstructuredPod}, metav1.CreateOptions{})
				//_, err := rc.dynamicCli.Resource().Namespace(reservation.Namespace).Create(context.TODO(), &runtime.Unstructured(podToCreate), interface{})
			} else {
				fmt.Printf("already found this pod %s - %s \n", podName, reservation.Namespace)
			}
		}
	}
	reservation.Status.ReservationStatus = resourcev1alpha1.ReservationStatusInProgress
}
func (rc *ReservationController) syncReservation(key string) error {
	namespace, name, err := cache.SplitMetaNamespaceKey(key)
	if err != nil {
		return fmt.Errorf("failed to get the namespace and name from key %s: %v", key, err)
	}
	reserve, err := rc.getReservation(namespace, name)
	if err != nil {
		return err
	}
	if reserve == nil {
		return nil
	}
	reserveCopy := reserve.DeepCopy()
	switch reserveCopy.Status.ReservationStatus {
	case resourcev1alpha1.ReservationStatusCreated:
		rc.handleNewlyCreatedReservation(reserveCopy)
	case resourcev1alpha1.ReservationStatusInProgress:
		rc.updateReservationStatusByCheckingPods(reserveCopy)
	case resourcev1alpha1.ReservationStatusFailed:
		{

		}
	case resourcev1alpha1.ReservationStatusTimeout:
		{

		}
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
		_, resourceId, replicaId := splitReservationPodName(pod.Name)
		reservation.Placeholders[pod.Name] = resourcev1alpha1.PodStatus{
			PodStatus: string(pod.Status.Phase),
		}
		if pod.Status.Phase == v1.PodRunning {
			cnt += 1
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
		reservation.Status.ReservationStatus = resourcev1alpha1.ReservationStatusCompleted
	}
}
func (rc *ReservationController) processNextItem() bool {
	key, quit := rc.queue.Get()
	if quit {
		return false
	}
	defer rc.queue.Done(key)
	err := rc.syncReservation(key.(string))
	if err == nil {
		rc.queue.Forget(key)
		return true
	}
	return true
}

func (rc *ReservationController) assemblyAPod(reservation *resourcev1alpha1.Reservation, replicaId int, resourceId int, mem string, core string) *v1.Pod {
	labels := reservation.Labels
	pod := &v1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      rc.getPlaceholderPodName(reservation, replicaId, resourceId),
			Namespace: reservation.Namespace,
			Labels:    labels,
		},
		Spec: v1.PodSpec{
			Containers: []v1.Container{
				{
					Name:            "pause",
					Image:           "k8s.gcr.io/pause:3.5",
					ImagePullPolicy: v1.PullIfNotPresent,
					Resources: v1.ResourceRequirements{
						Requests: v1.ResourceList{
							"memory": resource.MustParse(mem),
							"cpu":    resource.MustParse(core),
						},
					},
				},
			},
		},
	}
	return pod
}

func (rc *ReservationController) getReservationPods(reservation *resourcev1alpha1.Reservation) ([]*v1.Pod, error) {
	matchLabels := map[string]string{config.ReservationAppLabel: reservation.Name}
	selector := labels.SelectorFromSet(matchLabels)
	pods, err := rc.podLister.ByNamespace(reservation.Namespace).List(selector)
	var res []*v1.Pod
	for _, p := range pods {
		res = append(res, p.(*v1.Pod))
	}
	return res, err
}

func (rc *ReservationController) RunController() {
	fmt.Println("Launching reservation controller")
	defer utilruntime.HandleCrash()
	for rc.processNextItem() {
		fmt.Println("next loop")
	}
}

//+kubebuilder:rbac:groups=resource.scheduling.org,resources=reservations,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=resource.scheduling.org,resources=reservations/status,verbs=get;update;patch
//+kubebuilder:rbac:groups=resource.scheduling.org,resources=reservations/finalizers,verbs=update

// Reconcile is part of the main kubernetes reconciliation loop which aims to
// move the current state of the cluster closer to the desired state.
// TODO(user): Modify the Reconcile function to compare the state specified by
// the Reservation object against the actual cluster state, and then
// perform operations to make the cluster state reflect the state specified by
// the user.
//
// For more details, check Reconcile and its Result here:
// - https://pkg.go.dev/sigs.k8s.io/controller-runtime@v0.13.0/pkg/reconcile
//func (r *ReservationReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
//	_ = log.FromContext(ctx)
//
//	// TODO(user): your logic here
//
//	return ctrl.Result{}, nil
//}
//
//// SetupWithManager sets up the controller with the Manager.
//func (r *ReservationReconciler) SetupWithManager(mgr ctrl.Manager) error {
//	return ctrl.NewControllerManagedBy(mgr).
//		For(&resourcev1alpha1.Reservation{}).
//		Complete(r)
//}
//func getPlaceholderPodName(reservation *resourcev1alpha1.Reservation, replicaId int, resourceId int) string {
//	return reservation.Name + "_" + strconv.Itoa(resourceId) + "_" + strconv.Itoa(replicaId)
//}
//func (r *ReservationReconciler) reconcile(request reconcile.Request, reservation *resourcev1alpha1.Reservation) (*reconcile.Result, error) {
//	for _, request := range reservation.Spec.ResourceRequests {
//		for i := 0; i < request.Replica; i++ {
//			podName := getPlaceholderPodName(reservation, i, request.ResourceId)
//			pod := &corev1.Pod{}
//			err := r.Get(context.TODO(), types.NamespacedName{
//				Name:      podName,
//				Namespace: reservation.Namespace,
//			}, pod)
//			if err != nil && errors.IsNotFound(err) {
//				err = r.Create(context.TODO(), r.createAPod(reservation, i, request.ResourceId, request.Mem, request.Cpu))
//				if err != nil {
//					return &reconcile.Result{}, err
//				} else {
//					return nil, nil
//				}
//			} else if err != nil {
//				return &reconcile.Result{}, nil
//			} else {
//				podStatus, ok := reservation.Placeholders[podName]
//				if ok {
//					if podStatus.PodStatus != string(pod.Status.Phase) {
//						reservation.Placeholders[podName] = resourcev1alpha1.PodStatus{
//							PodStatus: string(pod.Status.Phase),
//						}
//					}
//				} else {
//					reservation.Placeholders[podName] = resourcev1alpha1.PodStatus{
//						PodStatus: string(pod.Status.Phase),
//					}
//				}
//			}
//		}
//	}
//	return &reconcile.Result{}, nil
//}
//
//func (r *ReservationReconciler) createAPod(reservation *resourcev1alpha1.Reservation, replicaId int, resourceId int, mem string, core string) *corev1.Pod {
//	labels := reservation.Labels
//	pod := &corev1.Pod{
//		ObjectMeta: metav1.ObjectMeta{
//			Name:      getPlaceholderPodName(reservation, replicaId, resourceId),
//			Namespace: reservation.Namespace,
//			Labels:    labels,
//		},
//		Spec: corev1.PodSpec{
//			Containers: []corev1.Container{
//				{
//					Name:            "pause",
//					Image:           "k8s.gcr.io/pause:3.5",
//					ImagePullPolicy: corev1.PullIfNotPresent,
//					Resources: corev1.ResourceRequirements{
//						Requests: corev1.ResourceList{
//							"memory": resource.MustParse(mem),
//							"cpu":    resource.MustParse(core),
//						},
//					},
//				},
//			},
//		},
//	}
//	return pod
//}
