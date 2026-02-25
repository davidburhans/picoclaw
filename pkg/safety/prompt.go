package safety

import (
	"fmt"
	"strings"
	"time"
)

func (f *Filter) GenerateContextPrompt() string {
	var parts []string

	if f.birthYear > 0 {
		age := time.Now().Year() - f.birthYear
		parts = append(parts, fmt.Sprintf("The user was born in %d (approximately %d years old).", f.birthYear, age))

		if f.isYoungUser() {
			parts = append(parts, "Use simple vocabulary and age-appropriate examples. Avoid mature topics.")
		} else if f.isTeenUser() {
			parts = append(parts, "Adjust communication style for a teenager. Be helpful but appropriate.")
		}
	}

	if f.level != LevelOff {
		parts = append(parts, fmt.Sprintf("Safety level: %s", f.level))

		switch f.level {
		case LevelLow:
			parts = append(parts, "Apply light content filtering. Block obviously harmful content.")
		case LevelMedium:
			parts = append(parts, "Apply moderate content filtering. Redirect inappropriate topics appropriately.")
		case LevelHigh:
			if f.isYoungUser() {
				parts = append(parts, "Apply strict filtering. For sensitive topics, suggest involving a parent or guardian.")
			} else {
				parts = append(parts, "Apply strict content filtering.")
			}
		}
	}

	if len(parts) == 0 {
		return ""
	}

	return "\n# User Context\n" + strings.Join(parts, "\n")
}
