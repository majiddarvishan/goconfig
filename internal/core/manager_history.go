package goconfig

import "github.com/majiddarvishan/goconfig/history"

// EnableHistory enables or disables recording new events.
func (m *Manager) EnableHistory(enabled bool) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.historyEnabled = enabled
}

// History returns independent events in chronological order.
func (m *Manager) History() []history.ChangeEvent {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.history.GetAll()
}

// GetHistory retains the original getter spelling.
// Deprecated: use History.
func (m *Manager) GetHistory() []history.ChangeEvent { return m.History() }

// HistoryByPath returns newest-first events for path, bounded by limit.
func (m *Manager) HistoryByPath(path string, limit int) []history.ChangeEvent {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.history.GetByPath(path, limit)
}

// GetHistoryByPath retains the original getter spelling.
// Deprecated: use HistoryByPath.
func (m *Manager) GetHistoryByPath(path string, limit int) []history.ChangeEvent {
	return m.HistoryByPath(path, limit)
}

// ClearHistory removes all recorded events.
func (m *Manager) ClearHistory() {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.history.Clear()
}

func (m *Manager) addHistoryEvent(event history.ChangeEvent) {
	if m.historyEnabled && m.history != nil {
		m.history.Add(event)
	}
}
