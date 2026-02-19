package config

import (
	"encoding/json"
	"testing"
)

func TestScheduleEntries_UnmarshalJSON(t *testing.T) {
	tests := []struct {
		name    string
		json    string
		wantErr bool
		wantMap map[string]string // simplified verification: map[instanceName]timezone
	}{
		{
			name: "Singleton (Old Format)",
			json: `{"timezone": "UTC", "rules": []}`,
			wantMap: map[string]string{
				"": "UTC",
			},
		},
		{
			name: "Map (New Format)",
			json: `{
				"work": {"timezone": "America/New_York", "rules": []},
				"personal": {"timezone": "America/Los_Angeles", "rules": []}
			}`,
			wantMap: map[string]string{
				"work":     "America/New_York",
				"personal": "America/Los_Angeles",
			},
		},
		{
			name: "Map with Default Key",
			json: `{
				"": {"timezone": "UTC", "rules": []},
				"work": {"timezone": "EST", "rules": []}
			}`,
			wantMap: map[string]string{
				"":     "UTC",
				"work": "EST",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var entries ScheduleEntries
			if err := json.Unmarshal([]byte(tt.json), &entries); (err != nil) != tt.wantErr {
				t.Errorf("UnmarshalJSON() error = %v, wantErr %v", err, tt.wantErr)
			}
			if !tt.wantErr {
				if len(entries) != len(tt.wantMap) {
					t.Errorf("Expected %d entries, got %d", len(tt.wantMap), len(entries))
				}
				for k, v := range tt.wantMap {
					got, ok := entries[k]
					if !ok {
						t.Errorf("Missing key %q", k)
						continue
					}
					if got.Timezone != v {
						t.Errorf("Key %q: timezone = %v, want %v", k, got.Timezone, v)
					}
				}
			}
		})
	}
}
