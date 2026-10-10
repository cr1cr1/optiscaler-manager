// Package launchercli routes game launches to the launcher owning the
// game's wine prefix (issue 060): a prefix managed by Bottles, Lutris,
// or Heroic is launched through that launcher's own CLI so the prefix
// keeps a single owner — umu-run never touches it. Command is the pure
// argv builder; Launch (Linux only) resolves the CLI and spawns it
// detached.
package launchercli

import "fmt"

// Opts describes a single owner-CLI launch (issue 060; logging added
// in issue 061). Source is the umu.Prefix source (bottles/lutris/
// heroic), Ref the prefix's OwnerRef (bottle name, Lutris slug, Heroic
// app name), Exe the game executable (Bottles only — Lutris and Heroic
// launch their configured game), Flatpak selects the flatpak-spawn
// argv form. LogDir/LogName mirror umu.LaunchOpts: a per-launch log
// file capturing the detached child's stdout/stderr.
type Opts struct {
	Source  string
	Ref     string
	Exe     string
	Flatpak bool
	LogDir  string
	LogName string
}

// Command builds the argv that launches via the owning launcher's CLI.
// source is the umu.Prefix source (bottles/lutris/heroic), ref the
// prefix's OwnerRef (bottle name, Lutris slug, Heroic app name), exe
// the game executable (Bottles only — Lutris and Heroic launch their
// configured game), flatpak selects the flatpak-spawn argv form.
//
// Steam compatdata, PlayOnLinux, and CrossOver prefixes are unroutable
// by design; the error names the owner and the remedy.
func Command(source, ref, exe string, flatpak bool) ([]string, error) {
	switch source {
	case "bottles":
		if ref == "" {
			return nil, fmt.Errorf("launchercli: bottles prefix without a bottle name")
		}
		if exe == "" {
			return nil, fmt.Errorf("launchercli: bottles launch needs the game executable")
		}
		argv := []string{"run", "-b", ref, "-e", exe}
		if flatpak {
			return append([]string{"flatpak", "run", "--command=bottles-cli", "com.usebottles.bottles"}, argv...), nil
		}
		return append([]string{"bottles-cli"}, argv...), nil
	case "lutris":
		if ref == "" {
			return nil, fmt.Errorf("launchercli: lutris prefix maps to no Lutris game")
		}
		uri := "lutris:rungame/" + ref
		if flatpak {
			return []string{"flatpak", "run", "net.lutris.Lutris", uri}, nil
		}
		return []string{"lutris", uri}, nil
	case "heroic":
		if ref == "" {
			return nil, fmt.Errorf("launchercli: heroic prefix maps to no Heroic game")
		}
		if flatpak {
			return []string{"flatpak", "run", "com.heroicgameslauncher.hgl", "launch", ref}, nil
		}
		return []string{"heroic", "launch", ref}, nil
	case "steam":
		return nil, fmt.Errorf("launchercli: Steam compatdata belongs to Steam — launch the Steam library row")
	case "playonlinux", "crossover":
		return nil, fmt.Errorf("launchercli: %s has no exe-based CLI — pin another prefix or set Launch via: umu", source)
	}
	return nil, fmt.Errorf("launchercli: prefix source %q is not launcher-owned", source)
}

// cliBinary is the native binary each backend needs on PATH (flatpak
// form needs only flatpak itself; the app presence is implied by the
// prefix living under its ~/.var/app root).
func cliBinary(source string, flatpak bool) string {
	if flatpak {
		return "flatpak"
	}
	switch source {
	case "bottles":
		return "bottles-cli"
	case "lutris":
		return "lutris"
	case "heroic":
		return "heroic"
	}
	return ""
}
