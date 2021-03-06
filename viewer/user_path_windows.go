package viewer

import (
	"os"
	"path/filepath"
)

func findUserConfigPath(appname string) string {
	home := os.Getenv("USERPROFILE")
	dir := os.Getenv("APPDATA")
	if dir == "" {
		dir = filepath.Join(home, "Application Data")
	}

	return filepath.Join(dir, appname)
}
