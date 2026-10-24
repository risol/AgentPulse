package application

import (
	"context"
	"errors"
)

const DefaultOTLPPort = 4318

var ErrInvalidReceiverPort = errors.New("OTLP receiver port must be between 1024 and 65535")

type ReceiverSettings struct {
	Port int `json:"port"`
}

type ReceiverSettingsRepository interface {
	LoadReceiverPort(context.Context) (*int, error)
	SaveReceiverPort(context.Context, int) error
}

// ReceiverSettingsService owns the persistent loopback receiver port and
// reports when a changed port needs to be applied to the running listener.
type ReceiverSettingsService struct {
	repository ReceiverSettingsRepository
}

func NewReceiverSettingsService(repository ReceiverSettingsRepository) *ReceiverSettingsService {
	return &ReceiverSettingsService{repository: repository}
}

func (s *ReceiverSettingsService) Load(ctx context.Context) (ReceiverSettings, error) {
	port, err := s.repository.LoadReceiverPort(ctx)
	if err != nil {
		return ReceiverSettings{}, err
	}
	if port == nil {
		return ReceiverSettings{Port: DefaultOTLPPort}, nil
	}
	if !validReceiverPort(*port) {
		return ReceiverSettings{}, ErrInvalidReceiverPort
	}
	return ReceiverSettings{Port: *port}, nil
}

func (s *ReceiverSettingsService) Update(ctx context.Context, port int) (ReceiverSettings, bool, error) {
	if err := ValidateReceiverPort(port); err != nil {
		return ReceiverSettings{}, false, err
	}
	current, err := s.Load(ctx)
	if err != nil {
		return ReceiverSettings{}, false, err
	}
	if current.Port == port {
		return current, false, nil
	}
	if err := s.repository.SaveReceiverPort(ctx, port); err != nil {
		return ReceiverSettings{}, false, err
	}
	return ReceiverSettings{Port: port}, true, nil
}

func ValidateReceiverPort(port int) error {
	if !validReceiverPort(port) {
		return ErrInvalidReceiverPort
	}
	return nil
}

func validReceiverPort(port int) bool { return port >= 1024 && port <= 65535 }
