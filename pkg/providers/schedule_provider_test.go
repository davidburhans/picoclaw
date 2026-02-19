package providers

import (
	"testing"
	"time"

	"github.com/sipeed/picoclaw/pkg/config"
)

func TestScheduleProvider_MatchRule(t *testing.T) {
	location := time.UTC

	tests := []struct {
		name     string
		now      time.Time
		schedule *config.ScheduleConfig
		want     bool
		wantProv string
	}{
		{
			name: "Match Day",
			now:  time.Date(2023, 10, 2, 10, 0, 0, 0, location), // Mon
			schedule: &config.ScheduleConfig{
				Rules: []config.ScheduleRule{
					{Days: []string{"mon"}, Provider: "p1"},
				},
			},
			want:     true,
			wantProv: "p1",
		},
		{
			name: "No Match Day",
			now:  time.Date(2023, 10, 3, 10, 0, 0, 0, location), // Tue
			schedule: &config.ScheduleConfig{
				Rules: []config.ScheduleRule{
					{Days: []string{"mon"}, Provider: "p1"},
				},
			},
			want:     false,
			wantProv: "",
		},
		{
			name: "Match Hours",
			now:  time.Date(2023, 10, 2, 10, 0, 0, 0, location), // 10:00
			schedule: &config.ScheduleConfig{
				Rules: []config.ScheduleRule{
					{Hours: &config.ScheduleHours{Start: "09:00", End: "11:00"}, Provider: "p1"},
				},
			},
			want:     true,
			wantProv: "p1",
		},
		{
			name: "No Match Hours",
			now:  time.Date(2023, 10, 2, 12, 0, 0, 0, location), // 12:00
			schedule: &config.ScheduleConfig{
				Rules: []config.ScheduleRule{
					{Hours: &config.ScheduleHours{Start: "09:00", End: "11:00"}, Provider: "p1"},
				},
			},
			want:     false,
			wantProv: "",
		},
		{
			name: "Match Overnight Late",
			now:  time.Date(2023, 10, 2, 23, 0, 0, 0, location), // 23:00
			schedule: &config.ScheduleConfig{
				Rules: []config.ScheduleRule{
					{Hours: &config.ScheduleHours{Start: "22:00", End: "06:00"}, Provider: "p1"},
				},
			},
			want:     true,
			wantProv: "p1",
		},
		{
			name: "Match Overnight Early",
			now:  time.Date(2023, 10, 3, 5, 0, 0, 0, location), // 05:00
			schedule: &config.ScheduleConfig{
				Rules: []config.ScheduleRule{
					{Hours: &config.ScheduleHours{Start: "22:00", End: "06:00"}, Provider: "p1"},
				},
			},
			want:     true,
			wantProv: "p1",
		},
		{
			name: "No Match Overnight",
			now:  time.Date(2023, 10, 2, 20, 0, 0, 0, location), // 20:00
			schedule: &config.ScheduleConfig{
				Rules: []config.ScheduleRule{
					{Hours: &config.ScheduleHours{Start: "22:00", End: "06:00"}, Provider: "p1"},
				},
			},
			want:     false,
			wantProv: "",
		},
		{
			name: "Match Weekday Alias",
			now:  time.Date(2023, 10, 2, 10, 0, 0, 0, location), // Mon
			schedule: &config.ScheduleConfig{
				Rules: []config.ScheduleRule{
					{Days: []string{"weekday"}, Provider: "p1"},
				},
			},
			want:     true,
			wantProv: "p1",
		},
		{
			name: "No Match Weekday Alias",
			now:  time.Date(2023, 10, 7, 10, 0, 0, 0, location), // Sat
			schedule: &config.ScheduleConfig{
				Rules: []config.ScheduleRule{
					{Days: []string{"weekday"}, Provider: "p1"},
				},
			},
			want:     false,
			wantProv: "",
		},
		{
			name: "Match Weekend Alias",
			now:  time.Date(2023, 10, 8, 10, 0, 0, 0, location), // Sun
			schedule: &config.ScheduleConfig{
				Rules: []config.ScheduleRule{
					{Days: []string{"weekend"}, Provider: "p1"},
				},
			},
			want:     true,
			wantProv: "p1",
		},
		{
			name: "No Match Weekend Alias",
			now:  time.Date(2023, 10, 6, 10, 0, 0, 0, location), // Fri
			schedule: &config.ScheduleConfig{
				Rules: []config.ScheduleRule{
					{Days: []string{"weekend"}, Provider: "p1"},
				},
			},
			want:     false,
			wantProv: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p := NewScheduleProvider(&config.Config{}, tt.schedule, location)

			rule, ok := p.matchRule(tt.now)
			if ok != tt.want {
				t.Fatalf("matchRule() ok = %v, want %v", ok, tt.want)
			}
			if ok && rule.Provider != tt.wantProv {
				t.Errorf("matchRule() provider = %v, want %v", rule.Provider, tt.wantProv)
			}
		})
	}
}
