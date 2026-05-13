package router

import "github.com/500tpig/grok-search-go/internal/capability"

type RouteDecision struct {
	Capability     capability.Kind           `json:"capability"`
	Provider       string                    `json:"provider"`
	Attempt        int                       `json:"attempt"`
	Status         string                    `json:"status"`
	LatencyMS      int64                     `json:"latency_ms,omitempty"`
	FallbackReason capability.FallbackReason `json:"fallback_reason,omitempty"`
	FallbackDetail string                    `json:"fallback_detail,omitempty"`
	SubAttempts    int                       `json:"sub_attempts,omitempty"`
}

type RouteTrace struct {
	FinalProvider     string          `json:"final_provider,omitempty"`
	FallbackTriggered bool            `json:"fallback_triggered"`
	AttemptsCount     int             `json:"attempts_count"`
	Decisions         []RouteDecision `json:"route_decision,omitempty"`
}

func (t RouteTrace) Compact() map[string]any {
	return map[string]any{
		"final_provider":     t.FinalProvider,
		"fallback_triggered": t.FallbackTriggered,
		"attempts_count":     t.AttemptsCount,
	}
}
