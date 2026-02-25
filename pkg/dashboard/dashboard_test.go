package dashboard

import (
	"testing"
)

func TestActivityBuffer(t *testing.T) {
	ab := NewActivityBuffer(3)

	ab.Add(map[string]interface{}{"id": 1})
	ab.Add(map[string]interface{}{"id": 2})
	ab.Add(map[string]interface{}{"id": 3})

	events := ab.GetEvents()
	if len(events) != 3 {
		t.Errorf("expected 3 events, got %d", len(events))
	}

	// Test overflow
	ab.Add(map[string]interface{}{"id": 4})
	events = ab.GetEvents()
	if len(events) != 3 {
		t.Errorf("expected 3 events after overflow, got %d", len(events))
	}

	if events[0]["id"] != 2 {
		t.Errorf("expected first event to be id 2, got %v", events[0]["id"])
	}
	if events[2]["id"] != 4 {
		t.Errorf("expected last event to be id 4, got %v", events[2]["id"])
	}
}

func TestGenerateSchema(t *testing.T) {
	schema := GenerateSchema()
	if schema == nil {
		t.Fatal("expected schema to be generated, got nil")
	}

	if schema["type"] != "object" {
		t.Errorf("expected schema type object, got %v", schema["type"])
	}

	properties := schema["properties"].(map[string]interface{})
	if _, ok := properties["agents"]; !ok {
		t.Error("expected 'agents' property in schema")
	}
}
