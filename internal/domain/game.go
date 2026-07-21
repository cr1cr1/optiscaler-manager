// Package domain defines the core types shared across optiscaler-manager:
// discovered games, upscaler components, resolved release assets, and the
// install manifest that the installer drives and the store persists.
//
// AI agents: read this package first. These types are the vocabulary every
// other module (discovery, gh, installer, store, gui) speaks; change them
// only with a milestone-level decision recorded in docs/.
package domain

// Store identifies the launcher/storefront a game was discovered from.
// StoreSteam is the zero value so Games built before multi-store discovery
// (and any unkeyed zero value) remain Steam games.
type Store int

const (
	StoreSteam Store = iota
	StoreEpic
	StoreGOG
	StoreManual
)

// String returns the display name of the store.
func (s Store) String() string {
	switch s {
	case StoreEpic:
		return "Epic"
	case StoreGOG:
		return "GOG"
	case StoreManual:
		return "Manual"
	default:
		return "Steam"
	}
}

// TitleSource records which identification rule produced a game's display
// title (v0.8). The strings are persisted in the games cache — they are a
// wire contract, change them only with a cache schema bump.
type TitleSource string

const (
	SourceOverride TitleSource = "override" // user-pinned (settings.title_overrides)
	SourceStoreID  TitleSource = "storeid"  // steam_appid.txt resolved via the store
	SourceGOGInfo  TitleSource = "goggame"  // goggame-*.info name
	SourceEGStore  TitleSource = "egstore"  // .egstore manifest DisplayName
	SourceUnity    TitleSource = "unity"    // Unity *_Data/app.info product
	SourceFuzzy    TitleSource = "fuzzy"    // normalized store-search match
	SourcePE       TitleSource = "pe"       // PE version resources
	SourceStem     TitleSource = "stem"     // exe filename stem
	SourceFolder   TitleSource = "folder"   // directory name (last resort)
)

// Game is a discovered game installation (M2a discovery produces these).
type Game struct {
	AppID       string
	Name        string
	InstallDir  string // canonical game root
	LibraryPath string

	Store        Store  // storefront the game was discovered from
	AppName      string // Epic launch AppName; "" for other stores
	ExePath      string // resolved main executable; "" when unknown
	CompatPrefix string // Proton prefix path (linux only); "" when absent

	// SteamAppID is the numeric Steam app id when known — from
	// steam_appid.txt, a store manifest, or an online match; "" when
	// unknown. TitleSource records which identification rule produced
	// Name (empty for store-manifest rows and pre-v0.8 paths).
	SteamAppID  string
	TitleSource TitleSource
}
