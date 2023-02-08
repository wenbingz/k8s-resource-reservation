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

package v1alpha1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

const (
	ReservationStatusInProgress = "InProgress"
	ReservationStatusCompleted  = "Completed"
	ReservationStatusFailed     = "Failed"
	ReservationStatusTimeout    = "Timeout"
	ReservationStatusUnknown    = "Unknown"
)

type ResourceRequest struct {
	Mem        string `json:mem`
	Cpu        string `json:cpu`
	Replica    int    `json:replica`
	ResourceId int    `json:rid`
}

type PodStatus struct {
	PodStatus string `json:podstatus`
}

// ReservationSpec defines the desired state of Reservation
type ReservationSpec struct {
	// INSERT ADDITIONAL SPEC FIELDS - desired state of cluster
	// Important: Run "make" to regenerate code after modifying this file

	// Foo is an example field of Reservation. Edit reservation_types.go to remove/update
	ResourceRequests []ResourceRequest `json:"resourcerequest,omitempty"`
}

// ReservationStatus defines the observed state of Reservation
type ReservationStatus struct {
	// INSERT ADDITIONAL STATUS FIELD - define observed state of cluster
	// Important: Run "make" to regenerate code after modifying this file
	status string `json:"status"`
}

//+kubebuilder:object:root=true
//+kubebuilder:subresource:status

// Reservation is the Schema for the reservations API
type Reservation struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec         ReservationSpec      `json:"spec,omitempty"`
	Status       ReservationStatus    `json:"status,omitempty"`
	Placeholders map[string]PodStatus `json:"placeholders,omitempty"`
}

//+kubebuilder:object:root=true

// ReservationList contains a list of Reservation
type ReservationList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []Reservation `json:"items"`
}

func init() {
	SchemeBuilder.Register(&Reservation{}, &ReservationList{})
}
