package nginx

import (
	"fmt"
	"os"
	"path/filepath"
)

// EnabledDir renvoie le dossier sites-enabled associé à un fichier rangé dans
// sites-available : son dossier frère (ex: .../sites-available/x.conf →
// .../sites-enabled). Surchargeable par l'appelant si la disposition diffère.
func EnabledDir(file string) string {
	return filepath.Join(filepath.Dir(filepath.Dir(file)), "sites-enabled")
}

// Enable active un fichier en créant, dans enabledDir, un lien symbolique
// portant son nom de base et pointant vers le fichier (chemin absolu).
// Idempotent : si le lien existe déjà et pointe vers le bon fichier, renvoie
// created=false sans erreur. Un nom déjà pris par autre chose est une erreur.
func Enable(file, enabledDir string) (link string, created bool, err error) {
	abs, err := filepath.Abs(file)
	if err != nil {
		return "", false, err
	}
	info, err := os.Stat(abs)
	if err != nil {
		return "", false, err
	}
	if info.IsDir() {
		return "", false, fmt.Errorf("%q est un dossier, pas un fichier", file)
	}
	if err := os.MkdirAll(enabledDir, 0o755); err != nil {
		return "", false, err
	}

	link = filepath.Join(enabledDir, filepath.Base(abs))
	switch target, rerr := os.Readlink(link); {
	case rerr == nil && target == abs:
		return link, false, nil // déjà activé vers la bonne cible
	case rerr == nil:
		return "", false, fmt.Errorf("%q existe déjà et pointe vers %s", link, target)
	}
	if _, lerr := os.Lstat(link); lerr == nil {
		return "", false, fmt.Errorf("%q existe déjà (n'est pas un lien)", link)
	}
	if err := os.Symlink(abs, link); err != nil {
		return "", false, err
	}
	return link, true, nil
}

// Disable désactive un fichier en supprimant son lien symbolique dans
// enabledDir. Par sécurité, refuse de retirer une entrée qui n'est pas un lien
// symbolique (pour ne jamais effacer un vrai fichier de configuration).
func Disable(file, enabledDir string) (string, error) {
	link := filepath.Join(enabledDir, filepath.Base(file))
	info, err := os.Lstat(link)
	if err != nil {
		if os.IsNotExist(err) {
			return "", fmt.Errorf("%q n'est pas activé", link)
		}
		return "", err
	}
	if info.Mode()&os.ModeSymlink == 0 {
		return "", fmt.Errorf("%q n'est pas un lien symbolique, suppression refusée", link)
	}
	if err := os.Remove(link); err != nil {
		return "", err
	}
	return link, nil
}
