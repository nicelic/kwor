// Keep non-English locale overlays shallow. Missing port-forward strings are
// resolved through the configured English fallback locale, while this typed
// record avoids recursively inferring the complete locale object six times.
const portForwardFallback: Record<string, string> = {}

export default portForwardFallback
