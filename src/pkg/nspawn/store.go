package nspawn

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"

	"github.com/PatriceAndoniaina/isolation-manager/src/pkg/container"
	apperrors "github.com/PatriceAndoniaina/isolation-manager/src/pkg/errors"
)

// fileStore persiste les métadonnées des conteneurs sous forme de fichiers JSON.
//
// La source de vérité applicative (port SSH alloué, limites, date de création)
// vit ici car machinectl ne conserve pas ces informations. Les fichiers sont
// écrits avec des permissions strictes (0600).
type fileStore struct {
	dir string
}

func newFileStore(dir string) *fileStore { return &fileStore{dir: dir} }

// path renvoie le chemin du fichier de métadonnées d'un conteneur.
func (s *fileStore) path(name string) string {
	return filepath.Join(s.dir, name+".json")
}

// save écrit (ou remplace) les métadonnées d'un conteneur.
func (s *fileStore) save(c *container.Container) error {
	if err := os.MkdirAll(s.dir, 0o700); err != nil {
		return err
	}
	data, err := json.MarshalIndent(c, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(s.path(c.Name), data, 0o600)
}

// load lit les métadonnées d'un conteneur, ErrNotFound s'il n'existe pas.
func (s *fileStore) load(name string) (*container.Container, error) {
	data, err := os.ReadFile(s.path(name))
	if err != nil {
		if os.IsNotExist(err) {
			return nil, apperrors.Wrap("load", name, apperrors.ErrNotFound)
		}
		return nil, err
	}
	var c container.Container
	if err := json.Unmarshal(data, &c); err != nil {
		return nil, err
	}
	return &c, nil
}

// delete supprime les métadonnées, ErrNotFound si absentes.
func (s *fileStore) delete(name string) error {
	err := os.Remove(s.path(name))
	if err != nil {
		if os.IsNotExist(err) {
			return apperrors.Wrap("delete", name, apperrors.ErrNotFound)
		}
		return err
	}
	return nil
}

// exists indique si un conteneur est connu du store.
func (s *fileStore) exists(name string) bool {
	_, err := os.Stat(s.path(name))
	return err == nil
}

// list renvoie tous les conteneurs persistés (répertoire absent = liste vide).
func (s *fileStore) list() ([]*container.Container, error) {
	entries, err := os.ReadDir(s.dir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	out := make([]*container.Container, 0, len(entries))
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".json") {
			continue
		}
		name := strings.TrimSuffix(e.Name(), ".json")
		c, err := s.load(name)
		if err != nil {
			return nil, err
		}
		out = append(out, c)
	}
	return out, nil
}
