package safety

import (
	"testing"
)

func TestFilter_BirthYear(t *testing.T) {
	tests := []struct {
		name      string
		birthYear int
		wantYoung bool
		wantTeen  bool
	}{
		{"born 2018", 2018, true, false},
		{"born 2015", 2015, true, false},
		{"born 2013", 2013, false, true},
		{"born 2010", 2010, false, true},
		{"born 1990", 1990, false, false},
		{"born 1980", 1980, false, false},
		{"no birth year", 0, false, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f := NewFilter("low", tt.birthYear)
			if got := f.isYoungUser(); got != tt.wantYoung {
				t.Errorf("isYoungUser() = %v, want %v", got, tt.wantYoung)
			}
			if got := f.isTeenUser(); got != tt.wantTeen {
				t.Errorf("isTeenUser() = %v, want %v", got, tt.wantTeen)
			}
		})
	}
}

func TestFilter_CheckContent(t *testing.T) {
	tests := []struct {
		name      string
		level     string
		birthYear int
		content   string
		wantBlock bool
	}{
		{"low blocks adult", "low", 1980, "violence and weapons", true},
		{"low passes normal", "low", 1980, "how to cook pasta", false},
		{"medium blocks more", "medium", 1980, "how to make a bomb", true},
		{"high young blocks teen topics", "high", 2015, "dating advice", true},
		{"off passes all", "off", 2015, "anything goes", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f := NewFilter(tt.level, tt.birthYear)
			blocked, _ := f.CheckContent(tt.content)
			if blocked != tt.wantBlock {
				t.Errorf("CheckContent() = %v, want %v", blocked, tt.wantBlock)
			}
		})
	}
}

func TestFilter_CheckResponse(t *testing.T) {
	f := NewFilter("off", 0)
	result := f.CheckResponse("Hello world")
	if !result.Safe {
		t.Error("Expected response to be safe when safety is off")
	}

	f = NewFilter("high", 2015)
	result = f.CheckResponse("Here is some dating advice")
	if result.NeedsApproval {
		t.Log("Correctly flagged for approval")
	}
}

func TestFilter_GenerateContextPrompt(t *testing.T) {
	f := NewFilter("low", 2015)
	prompt := f.GetSystemPrompt()
	if prompt == "" {
		t.Error("Expected non-empty prompt for young user with safety")
	}

	f = NewFilter("off", 0)
	prompt = f.GetSystemPrompt()
	if prompt != "" {
		t.Error("Expected empty prompt when no settings")
	}
}
