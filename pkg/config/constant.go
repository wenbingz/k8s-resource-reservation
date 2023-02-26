package config

const (
	LabelAnnotationPrefix       = "reservation.org/"
	ReservationAppLabel         = LabelAnnotationPrefix + "reservation-app"
	ReservationResourceId       = LabelAnnotationPrefix + "resource-id"
	ReservationReplicaId        = LabelAnnotationPrefix + "replica-id"
	ReservationRole             = LabelAnnotationPrefix + "reservation-role"
	PlaceholderRole             = "placeholder-pod"
	PlaceholderPodPriorityClass = "low-priority-apps"
)
