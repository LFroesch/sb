package userdirs

import (
	"os"
	"path/filepath"
	"strings"
)

func HomeDir() string {
	if home, ok := os.LookupEnv("HOME"); ok {
		home = strings.TrimSpace(home)
		if home == "" {
			return ""
		}
		return home
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return ""
	}
	return strings.TrimSpace(home)
}

func ConfigHome() string {
	if dir := strings.TrimSpace(os.Getenv("XDG_CONFIG_HOME")); dir != "" {
		return dir
	}
	if home := HomeDir(); home != "" {
		return filepath.Join(home, ".config")
	}
	return ""
}

func DataHome() string {
	if dir := strings.TrimSpace(os.Getenv("XDG_DATA_HOME")); dir != "" {
		return dir
	}
	if home := HomeDir(); home != "" {
		return filepath.Join(home, ".local", "share")
	}
	return ""
}

func StateHome() string {
	if dir := strings.TrimSpace(os.Getenv("XDG_STATE_HOME")); dir != "" {
		return dir
	}
	if home := HomeDir(); home != "" {
		return filepath.Join(home, ".local", "state")
	}
	return ""
}
