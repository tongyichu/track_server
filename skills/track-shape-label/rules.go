// Package trackshape embeds the shared shape recognition rules for Kimi.
package trackshape

import _ "embed"

// Prompt is the same rule text referenced by SKILL.md.
//
//go:embed references/shape-rules.txt
var Prompt string

// LearningPrompt defines how admin corrections become bounded visual examples.
//
//go:embed references/learning-rules.txt
var LearningPrompt string
