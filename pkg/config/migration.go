// PicoClaw - Ultra-lightweight personal AI agent
// License: MIT
//
// Copyright (c) 2026 PicoClaw contributors

package config

import (
	"strings"
)

// ExtractProtocol extracts the protocol and model ID from a model string.
func ExtractProtocol(model string) (string, string) {
	protocol, modelID, found := strings.Cut(model, "/")
	if !found {
		return "openai", model
	}
	return protocol, modelID
}
