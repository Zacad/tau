package types

// GetSupportedThinkingLevels returns the thinking levels supported by a model.
// Levels mapped to empty string ("") in ThinkingLevelMap are excluded.
// xhigh requires explicit support (must be in the map with non-empty value).
func (m *Model) GetSupportedThinkingLevels() []ThinkingLevel {
	if !m.Reasoning {
		return []ThinkingLevel{ThinkingOff}
	}

	all := AllThinkingLevels()
	supported := make([]ThinkingLevel, 0, len(all))
	for _, level := range all {
		if level == ThinkingXHigh {
			// xhigh needs explicit support
			if m.ThinkingLevelMap != nil {
				if v, ok := m.ThinkingLevelMap[string(level)]; ok && v != "" {
					supported = append(supported, level)
				}
			}
			continue
		}
		// For other levels, check if explicitly disabled
		if m.ThinkingLevelMap != nil {
			if v, ok := m.ThinkingLevelMap[string(level)]; ok && v == "" {
				continue // explicitly unsupported
			}
		}
		supported = append(supported, level)
	}
	return supported
}

// MapThinkingLevel returns the provider-specific value for a thinking level.
// If no mapping exists, returns the level string as-is.
func (m *Model) MapThinkingLevel(level ThinkingLevel) string {
	if m.ThinkingLevelMap == nil {
		return string(level)
	}
	if v, ok := m.ThinkingLevelMap[string(level)]; ok && v != "" {
		return v
	}
	return string(level)
}
