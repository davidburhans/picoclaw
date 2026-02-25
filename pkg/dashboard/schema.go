package dashboard

import (
	"reflect"
	"strings"

	"github.com/sipeed/picoclaw/pkg/config"
)

// GenerateSchema creates a simple JSON schema for the Config struct.
func GenerateSchema() map[string]interface{} {
	return reflectTypeToSchema(reflect.TypeOf(config.Config{}))
}

func reflectTypeToSchema(t reflect.Type) map[string]interface{} {
	if t.Kind() == reflect.Ptr {
		t = t.Elem()
	}

	schema := map[string]interface{}{
		"type": stringKind(t.Kind()),
	}

	switch t.Kind() {
	case reflect.Struct:
		properties := make(map[string]interface{})
		for i := 0; i < t.NumField(); i++ {
			field := t.Field(i)
			jsonTag := field.Tag.Get("json")
			if jsonTag == "" || jsonTag == "-" {
				continue
			}
			name := strings.Split(jsonTag, ",")[0]

			prop := reflectTypeToSchema(field.Type)
			envTag := field.Tag.Get("env")
			if envTag != "" {
				prop["description"] = "Environment variable: " + envTag
			}

			properties[name] = prop
		}
		schema["properties"] = properties
		schema["type"] = "object"

	case reflect.Slice, reflect.Array:
		schema["type"] = "array"
		schema["items"] = reflectTypeToSchema(t.Elem())

	case reflect.Map:
		schema["type"] = "object"
		schema["additionalProperties"] = reflectTypeToSchema(t.Elem())
	}

	return schema
}

func stringKind(k reflect.Kind) string {
	switch k {
	case reflect.Bool:
		return "boolean"
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64,
		reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64, reflect.Float32, reflect.Float64:
		return "number"
	case reflect.String:
		return "string"
	default:
		return "object"
	}
}
