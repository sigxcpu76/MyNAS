package domain

import "gopkg.in/yaml.v3"

// DashboardWidgetPreference represents the placement of a single widget on the dashboard.
type DashboardWidgetPreference struct {
	ID   string `json:"id" yaml:"id"`
	Type string `json:"type" yaml:"type"`
	Lane int32  `json:"lane" yaml:"lane"`
}

// DashboardPreferences holds layout and column configuration for the dashboard.
type DashboardPreferences struct {
	ColumnCount int32                       `json:"columnCount" yaml:"columnCount"`
	Layout      []DashboardWidgetPreference `json:"layout" yaml:"layout"`
}

func (d *DashboardPreferences) UnmarshalYAML(value *yaml.Node) error {
	type rawType struct {
		ColumnCount      int32                       `yaml:"columnCount"`
		ColumnCountSnake int32                       `yaml:"column_count"`
		Layout           []DashboardWidgetPreference `yaml:"layout"`
	}
	var raw rawType
	if err := value.Decode(&raw); err != nil {
		return err
	}
	d.Layout = raw.Layout
	d.ColumnCount = raw.ColumnCount
	if d.ColumnCount == 0 && raw.ColumnCountSnake != 0 {
		d.ColumnCount = raw.ColumnCountSnake
	}
	if d.ColumnCount <= 0 {
		d.ColumnCount = 3
	}
	return nil
}

// UserPreferences stores all per-user configuration saved in ~/.config/nas_preferences.yaml.
type UserPreferences struct {
	Dashboard DashboardPreferences `json:"dashboard" yaml:"dashboard"`
	Custom    map[string]any       `json:"custom,omitempty" yaml:",inline"`
}
