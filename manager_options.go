package goconfig

import "fmt"

const DefaultHistoryCapacity = 1000

type managerOptions struct {
	historyCapacity int
}

// ManagerOption customizes Manager construction.
type ManagerOption func(*managerOptions) error

// WithHistoryCapacity sets the maximum number of retained history events.
func WithHistoryCapacity(capacity int) ManagerOption {
	return func(options *managerOptions) error {
		if capacity <= 0 {
			return fmt.Errorf("history capacity must be greater than zero")
		}
		options.historyCapacity = capacity
		return nil
	}
}

func applyManagerOptions(options []ManagerOption) (managerOptions, error) {
	settings := managerOptions{historyCapacity: DefaultHistoryCapacity}
	for index, option := range options {
		if option == nil {
			return managerOptions{}, fmt.Errorf("manager option %d is nil", index)
		}
		if err := option(&settings); err != nil {
			return managerOptions{}, fmt.Errorf("manager option %d: %w", index, err)
		}
	}
	return settings, nil
}
