package services

import "apartments-clone-server/MeskenyGPT/ai"

// MeskenyGPTService is an optional pointer to the MeskenyGPT backend.
// When set (in main.go), notification workers can use it to generate smarter,
// localized copy with safe fallbacks.
var MeskenyGPTService ai.Service

