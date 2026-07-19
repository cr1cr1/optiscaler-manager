package domain

// Kind identifies an upscaler component family detected in a game directory.
type Kind int

const (
	KindDLSS Kind = iota
	KindDLSSFG
	KindFSR
	KindXeSS
)

// String returns the human-facing name of the component family.
func (k Kind) String() string {
	switch k {
	case KindDLSS:
		return "DLSS"
	case KindDLSSFG:
		return "DLSS-FG"
	case KindFSR:
		return "FSR"
	case KindXeSS:
		return "XeSS"
	default:
		return "Unknown"
	}
}

// Component is a detected upscaler DLL in a game directory. The classifier
// reports kind and DLL filename only (docs/scope.md: PE version display cut).
type Component struct {
	Kind Kind
	DLL  string
}

// ResolvedAsset is the concrete release asset a requested version resolved
// to. It is recorded separately from the requested version so a rate-limit
// fallback or explicit substitution never silently changes what the user
// asked for (docs/safety.md).
type ResolvedAsset struct {
	AssetName string `json:"asset_name"`
	Version   string `json:"version"`
	SHA256    string `json:"sha256"`
}
