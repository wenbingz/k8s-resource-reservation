package v1alpha1

import (
	"context"
	"k8s.io/client-go/rest"
)

type ReservationInterface interface {
	Create(ctx context.Context, reservation *Reservation) (*Reservation, error)
	Update(ctx context.Context, reservation *Reservation) (*Reservation, error)
	UpdateStatus(ctx context.Context, reservation *Reservation) (*Reservation, error)
}

type reservationClient struct {
	client rest.Interface
	ns     string
}
