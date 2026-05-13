package capability

type Outcome string

const (
	OK        Outcome = "OK"
	Empty     Outcome = "Empty"
	Transient Outcome = "Transient"
	Permanent Outcome = "Permanent"
	Canceled  Outcome = "Canceled"
)

func (o Outcome) Status() string {
	switch o {
	case OK:
		return "ok"
	case Empty:
		return "empty"
	case Canceled:
		return "canceled"
	default:
		return "error"
	}
}

func (o Outcome) ShouldFallback() bool {
	return o == Empty || o == Transient
}

type FallbackReason string

const (
	ReasonNone          FallbackReason = ""
	ReasonNoContent     FallbackReason = "no_content"
	ReasonUpstreamError FallbackReason = "upstream_error"
	ReasonTimeout       FallbackReason = "timeout"
	ReasonRateLimited   FallbackReason = "rate_limited"
	ReasonUnconfigured  FallbackReason = "unconfigured"
	ReasonDisabled      FallbackReason = "disabled"
	ReasonUserDisabled  FallbackReason = "user_disabled"
	ReasonNotApplicable FallbackReason = "not_applicable"
)
