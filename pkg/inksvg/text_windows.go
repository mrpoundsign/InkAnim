//go:build windows

package inksvg

import (
	"os"
	"path/filepath"

	"golang.org/x/sys/windows/registry"
)

func scanPlatformFontRegistry(fm *FontManager) {
	windir := os.Getenv("WINDIR")
	if windir == "" {
		windir = "C:\\Windows"
	}
	defaultFontsDir := filepath.Join(windir, "Fonts")

	localApp := os.Getenv("LOCALAPPDATA")
	var userFontsDir string
	if localApp != "" {
		userFontsDir = filepath.Join(localApp, "Microsoft", "Windows", "Fonts")
	}

	searchDirs := fm.systemDirs
	if len(searchDirs) == 0 {
		searchDirs = []string{defaultFontsDir}
		if userFontsDir != "" {
			searchDirs = append(searchDirs, userFontsDir)
		}
	}

	readKey := func(root registry.Key, path string) map[string]string {
		k, err := registry.OpenKey(root, path, registry.READ)
		if err != nil {
			return nil
		}
		defer k.Close()

		names, err := k.ReadValueNames(-1)
		if err != nil {
			return nil
		}

		entries := make(map[string]string, len(names))
		for _, name := range names {
			val, _, err := k.GetStringValue(name)
			if err == nil && val != "" {
				entries[name] = val
			}
		}
		return entries
	}

	if sysEntries := readKey(registry.LOCAL_MACHINE, `SOFTWARE\Microsoft\Windows NT\CurrentVersion\Fonts`); len(sysEntries) > 0 {
		processRegistryFontEntries(sysEntries, searchDirs, fm.fileMap)
	}

	if userEntries := readKey(registry.CURRENT_USER, `SOFTWARE\Microsoft\Windows NT\CurrentVersion\Fonts`); len(userEntries) > 0 {
		processRegistryFontEntries(userEntries, searchDirs, fm.fileMap)
	}
}
