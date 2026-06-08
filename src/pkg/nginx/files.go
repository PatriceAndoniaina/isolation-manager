package nginx

import (
	"io/fs"
	"path/filepath"
	"sort"
	"strings"
)

// confExt est l'extension reconnue des fichiers de configuration nginx.
const confExt = ".conf"

// ListFiles parcourt dir récursivement et renvoie, triés, les fichiers de
// configuration nginx (extension .conf). Une erreur d'accès au dossier est
// remontée ; un dossier sans .conf donne une liste vide.
func ListFiles(dir string) ([]string, error) {
	var files []string
	err := filepath.WalkDir(dir, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if !d.IsDir() && strings.HasSuffix(d.Name(), confExt) {
			files = append(files, path)
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	sort.Strings(files)
	return files, nil
}
