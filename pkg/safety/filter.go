package safety

import (
	"fmt"
	"strings"
	"time"
)

const (
	LevelOff    = "off"
	LevelLow    = "low"
	LevelMedium = "medium"
	LevelHigh   = "high"
)

var adultKeywords = []string{
	"violence", "weapons", "drugs", "alcohol", "tobacco",
	"gambling", "hate", "discrimination", "self-harm",
	"explicit", "pornography", "sexual",
}

var mediumBlockKeywords = []string{
	"suicide", "murder", "kill", "attack", "bomb",
	"hack", "steal", "fraud", "scam",
}

type Filter struct {
	level     string
	birthYear int
}

func NewFilter(level string, birthYear int) *Filter {
	if level == "" {
		level = LevelOff
	}
	return &Filter{
		level:     level,
		birthYear: birthYear,
	}
}

func (f *Filter) Level() string {
	return f.level
}

func (f *Filter) BirthYear() int {
	return f.birthYear
}

func (f *Filter) isYoungUser() bool {
	if f.birthYear == 0 {
		return false
	}
	age := time.Now().Year() - f.birthYear
	return age < 13
}

func (f *Filter) isTeenUser() bool {
	if f.birthYear == 0 {
		return false
	}
	age := time.Now().Year() - f.birthYear
	return age >= 13 && age < 18
}

func (f *Filter) shouldBlock() bool {
	return f.level != LevelOff
}

func (f *Filter) CheckContent(content string) (blocked bool, reason string) {
	if !f.shouldBlock() {
		return false, ""
	}

	contentLower := strings.ToLower(content)

	if f.level == LevelLow {
		for _, kw := range adultKeywords {
			if strings.Contains(contentLower, kw) {
				return true, "content blocked by safety filter (low)"
			}
		}
	}

	if f.level == LevelMedium || f.level == LevelHigh {
		for _, kw := range adultKeywords {
			if strings.Contains(contentLower, kw) {
				return true, "content blocked by safety filter (medium/high)"
			}
		}
		for _, kw := range mediumBlockKeywords {
			if strings.Contains(contentLower, kw) {
				return true, "content blocked by safety filter (medium/high)"
			}
		}
	}

	if f.level == LevelHigh && f.isYoungUser() {
		teenOnlyTopics := []string{"dating", "romance", "sex", "politics", "religion"}
		for _, kw := range teenOnlyTopics {
			if strings.Contains(contentLower, kw) {
				return true, "content requires parent approval (high safety for young user)"
			}
		}
	}

	return false, ""
}

func (f *Filter) RequiresApproval() bool {
	return f.level == LevelHigh && f.isYoungUser()
}

type CheckResult struct {
	Safe           bool   // true if content is safe to send
	Blocked        bool   // true if content should be blocked entirely
	Rewrite        bool   // true if content needs to be rewritten
	NeedsApproval  bool   // true if content needs parent approval
	Reason         string // explanation of the decision
	Original       string // original response
	Rewritten      string // rewritten response (if Rewrite is true)
	BlockedMessage string // message to show user instead of blocked content
}

func (f *Filter) CheckResponse(response string) *CheckResult {
	result := &CheckResult{
		Original: response,
		Safe:     true,
	}

	// If safety is off, pass everything through
	if f.level == LevelOff {
		return result
	}

	// First: keyword-based quick check
	blocked, reason := f.CheckContent(response)
	if blocked {
		result.Safe = false
		result.Blocked = true
		result.Reason = reason
		result.BlockedMessage = f.getBlockedMessage(reason)
		return result
	}

	// For high safety with young users, flag for approval
	if f.level == LevelHigh && f.isYoungUser() {
		sensitiveTopics := []string{"dating", "romance", "sex", "politics", "religion", "death", "grief"}
		contentLower := strings.ToLower(response)
		for _, topic := range sensitiveTopics {
			if strings.Contains(contentLower, topic) {
				result.Safe = true // Still safe but flag for review
				result.NeedsApproval = true
				result.Reason = "Sensitive topic for young user - parent review recommended"
				return result
			}
		}
	}

	// For medium/high with older users, do additional context-aware check
	if f.level == LevelMedium || f.level == LevelHigh {
		if f.needsLLMCheck(response) {
			result.NeedsApproval = true
			result.Reason = "Content may need review - using LLM safety check recommended"
		}
	}

	return result
}

func (f *Filter) needsLLMCheck(response string) bool {
	// Simple heuristics for when LLM check might be needed
	// This is a placeholder - in production, you'd call an LLM here
	contentLower := strings.ToLower(response)
	ambiguousPhrases := []string{
		"in my opinion", "some people believe", "it depends",
		"you should ask", "talk to your parents", "consult an adult",
	}
	for _, phrase := range ambiguousPhrases {
		if strings.Contains(contentLower, phrase) {
			return true
		}
	}
	return false
}

func (f *Filter) getBlockedMessage(reason string) string {
	if f.isYoungUser() {
		return "I can't share that information with you. Ask a parent or guardian if you'd like to know more about this topic."
	}
	return "This content has been filtered for safety. Please try a different topic."
}

func (f *Filter) GetSystemPrompt() string {
	var parts []string

	if f.birthYear > 0 {
		age := time.Now().Year() - f.birthYear
		parts = append(parts, fmt.Sprintf("The user was born in %d (approximately %d years old).", f.birthYear, age))

		if f.isYoungUser() {
			parts = append(parts, "IMPORTANT: This is a young child. Use simple vocabulary, short sentences, and age-appropriate examples.")
		} else if f.isTeenUser() {
			parts = append(parts, "IMPORTANT: This is a teenager. Be helpful but mindful of age-appropriate content.")
		}
	}

	if f.level != LevelOff {
		parts = append(parts, fmt.Sprintf("Safety filter level: %s", f.level))
	}

	if len(parts) > 0 {
		return "## Safety Context\n" + strings.Join(parts, "\n")
	}
	return ""
}
