package config

const (
	LabelAnnotationPrefix       = "reservation.org/"
	ReservationAppLabel         = LabelAnnotationPrefix + "reservation-app"
	ReservationResourceId       = LabelAnnotationPrefix + "resource-id"
	ReservationReplicaId        = LabelAnnotationPrefix + "replica-id"
	PlaceholderPodPriorityClass = "low-priority-apps"
)
