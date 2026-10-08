package account

// ApplyStrategy gives one pool the strategy a provider setting names,
// falling back to the strategy the manager was built with for an empty
// name.
func (m *Manager) ApplyStrategy(providerID, name string) error {
	strategy, err := StrategyNamed(name)
	if err != nil {
		return err
	}
	if name == "" {
		m.mu.RLock()
		strategy = m.options.Strategy
		m.mu.RUnlock()
		if strategy == nil {
			strategy, _ = StrategyNamed(StrategyLeastLoaded)
		}
	}
	m.GetPool(providerID).SetStrategy(strategy)
	return nil
}
