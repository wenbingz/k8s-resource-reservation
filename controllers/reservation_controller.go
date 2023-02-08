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

package controllers

import (
	"context"
	resourcev1alpha1 "github.com/wenbingz/k8s-resource-reservation/pkg/apis/reservation/v1alpha1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
	"strconv"
)

// ReservationReconciler reconciles a Reservation object
type ReservationReconciler struct {
	client.Client
	Scheme *runtime.Scheme
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
func (r *ReservationReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	_ = log.FromContext(ctx)

	// TODO(user): your logic here

	return ctrl.Result{}, nil
}

// SetupWithManager sets up the controller with the Manager.
func (r *ReservationReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&resourcev1alpha1.Reservation{}).
		Complete(r)
}
func getPlaceholderPodName(reservation *resourcev1alpha1.Reservation, replicaId int, resourceId int) string {
	return reservation.Name + "_" + strconv.Itoa(resourceId) + "_" + strconv.Itoa(replicaId)
}
func (r *ReservationReconciler) reconcile(request reconcile.Request, reservation *resourcev1alpha1.Reservation) (*reconcile.Result, error) {
	for _, request := range reservation.Spec.ResourceRequests {
		for i := 0; i < request.Replica; i++ {
			podName := getPlaceholderPodName(reservation, i, request.ResourceId)
			pod := &corev1.Pod{}
			err := r.Get(context.TODO(), types.NamespacedName{
				Name:      podName,
				Namespace: reservation.Namespace,
			}, pod)
			if err != nil && errors.IsNotFound(err) {
				err = r.Create(context.TODO(), r.createAPod(reservation, i, request.ResourceId, request.Mem, request.Cpu))
				if err != nil {
					return &reconcile.Result{}, err
				} else {
					return nil, nil
				}
			} else if err != nil {
				return &reconcile.Result{}, nil
			} else {
				podStatus, ok := reservation.Placeholders[podName]
				if ok {
					if podStatus.PodStatus != string(pod.Status.Phase) {
						reservation.Placeholders[podName] = resourcev1alpha1.PodStatus{
							PodStatus: string(pod.Status.Phase),
						}
					}
				} else {
					reservation.Placeholders[podName] = resourcev1alpha1.PodStatus{
						PodStatus: string(pod.Status.Phase),
					}
				}
			}
		}
	}
}

func (r *ReservationReconciler) createAPod(reservation *resourcev1alpha1.Reservation, replicaId int, resourceId int, mem string, core string) *corev1.Pod {
	labels := reservation.Labels
	pod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      getPlaceholderPodName(reservation, replicaId, resourceId),
			Namespace: reservation.Namespace,
			Labels:    labels,
		},
		Spec: corev1.PodSpec{
			Containers: []corev1.Container{
				{
					Name:            "pause",
					Image:           "k8s.gcr.io/pause:3.5",
					ImagePullPolicy: corev1.PullIfNotPresent,
					Resources: corev1.ResourceRequirements{
						Requests: corev1.ResourceList{
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
