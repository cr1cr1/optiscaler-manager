//go:build linux

package umu

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"

	"github.com/rs/zerolog/log"
)

// KillWineserver best-effort-stops the wineserver serving prefix using
// the runner at protonPath (issue 069): process-group kills miss wine
// processes — wineserver re-sessions its clients, so a stopped game
// keeps running until the prefix's wineserver is killed. Both runner
// layouts are tried (GE-Proton-style bin/, UMU-Proton files/bin/).
// Silent no-op when no wineserver binary exists.
func KillWineserver(ctx context.Context, protonPath, prefix string) {
	if protonPath == "" || prefix == "" {
		return
	}
	for _, rel := range []string{"bin/wineserver", "files/bin/wineserver"} {
		ws := filepath.Join(protonPath, rel)
		st, err := os.Stat(ws)
		if err != nil || st.IsDir() || st.Mode()&0o111 == 0 {
			continue
		}
		cmd := exec.CommandContext(ctx, ws, "-k")
		cmd.Env = append(os.Environ(), "WINEPREFIX="+prefix)
		if err := cmd.Run(); err != nil {
			log.Debug().Err(err).Str("wineserver", ws).Str("prefix", prefix).Msg("wineserver -k failed")
		}
		return
	}
}
