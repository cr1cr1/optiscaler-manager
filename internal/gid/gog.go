package gid

import (
	"encoding/json"
	"fmt"
	"io"
	"strings"
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
