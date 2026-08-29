package goconfig

import "fmt"

// SetValidationService configures fail-closed external validation.
func (m *Manager) SetValidationService(service *ValidationService) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.validationService = service
}

// RegisterValidator validates and registers a pre-commit validator path.
func (m *Manager) RegisterValidator(path string, validator Validator) error {
	if validator == nil {
		return fmt.Errorf("validator cannot be nil")
	}
	segments, err := parseJSONPointer(path)
	if err != nil {
		return &PathError{Path: path, Err: ErrInvalidPath, Msg: err.Error()}
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	m.customValidator.AddValidator(buildJSONPointer(segments), validator)
	return nil
}

// AddValidator retains the original no-error registration API.
// Deprecated: use RegisterValidator.
func (m *Manager) AddValidator(path string, validator Validator) {
	_ = m.RegisterValidator(path, validator)
}

// CustomValidator returns the Manager's synchronized validator registry.
func (m *Manager) CustomValidator() *CustomValidator { return m.customValidator }

// GetCustomValidator retains the original getter spelling.
// Deprecated: use CustomValidator.
func (m *Manager) GetCustomValidator() *CustomValidator { return m.CustomValidator() }
