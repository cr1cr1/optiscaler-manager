package discovery

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/rs/zerolog/log"
)

// GOGGameInfo is one parsed goggame-<id>.info file. These ship inside every
// GOG Galaxy-installed game directory and describe how to launch the game.
type GOGGameInfo struct {
	GameID    string
	Name      string
	PlayTasks []GOGPlayTask
}

// GOGPlayTask is one launchable entry in a goggame info file.
type GOGPlayTask struct {
	Name       string
	Path       string
	WorkingDir string
	IsPrimary  bool
	Category   string
}

type gogGameInfoJSON struct {
	GameID    string `json:"gameId"`
	Name      string `json:"name"`
	PlayTasks []struct {
		Name       string `json:"name"`
		Path       string `json:"path"`
		WorkingDir string `json:"workingDir"`
		IsPrimary  bool   `json:"isPrimary"`
		Category   string `json:"category"`
	} `json:"playTasks"`
}

// ParseGOGGameInfo parses one goggame-<id>.info JSON document from r.
func ParseGOGGameInfo(r io.Reader) (GOGGameInfo, error) {
	var raw gogGameInfoJSON
	if err := json.NewDecoder(r).Decode(&raw); err != nil {
		return GOGGameInfo{}, fmt.Errorf("goggame info: %w", err)
	}
	info := GOGGameInfo{GameID: raw.GameID, Name: raw.Name}
	for _, pt := range raw.PlayTasks {
		info.PlayTasks = append(info.PlayTasks, GOGPlayTask{
			Name:       pt.Name,
			Path:       pt.Path,
			WorkingDir: pt.WorkingDir,
			IsPrimary:  pt.IsPrimary,
			Category:   pt.Category,
		})
	}
	return info, nil
}

// PrimaryExe returns the path of the task that launches the game: the
// primary task when flagged, else the first "game"-category task, else the
// first task carrying a path. "" when no task qualifies.
func (info GOGGameInfo) PrimaryExe() string {
	for _, pt := range info.PlayTasks {
		if pt.IsPrimary && pt.Path != "" {
			return pt.Path
		}
	}
	for _, pt := range info.PlayTasks {
		if strings.EqualFold(pt.Category, "game") && pt.Path != "" {
			return pt.Path
		}
	}
	for _, pt := range info.PlayTasks {
		if pt.Path != "" {
			return pt.Path
		}
	}
	return ""
}

// GOGExePath locates the goggame-*.info file inside gameDir and resolves the
// game's primary executable to an absolute path that exists on disk.
// Windows-style separators in task paths are normalised. "" when no info
// file or executable can be resolved.
func GOGExePath(gameDir string) string {
	infos, err := filepath.Glob(filepath.Join(gameDir, "goggame-*.info"))
	if err != nil || len(infos) == 0 {
		return ""
	}
	f, err := os.Open(infos[0])
	if err != nil {
		return ""
	}
	defer func() { _ = f.Close() }()
	info, err := ParseGOGGameInfo(f)
	if err != nil {
		log.Debug().Err(err).Str("info", infos[0]).Msg("unparseable goggame info")
		return ""
	}
	rel := info.PrimaryExe()
	if rel == "" {
		return ""
	}
	rel = strings.ReplaceAll(rel, `\`, string(filepath.Separator))
	candidates := []string{filepath.Join(gameDir, rel)}
	for _, pt := range info.PlayTasks {
		if pt.WorkingDir != "" && pt.WorkingDir != "." {
			wd := strings.ReplaceAll(pt.WorkingDir, `\`, string(filepath.Separator))
			candidates = append(candidates, filepath.Join(gameDir, wd, filepath.Base(rel)))
		}
	}
	for _, c := range candidates {
		if st, err := os.Stat(c); err == nil && !st.IsDir() {
			return c
		}
	}
	log.Debug().Str("dir", gameDir).Str("rel", rel).Msg("goggame primary exe not found on disk")
	return ""
}
