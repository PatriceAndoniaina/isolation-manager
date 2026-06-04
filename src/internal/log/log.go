// Package log fournit un logger structuré (logrus) partagé par l'application.
//
// Il vit dans internal/ car c'est un utilitaire transverse : cmd/ et pkg/
// l'utilisent pour produire un audit trail cohérent de toutes les opérations
// système (exigence d'auditabilité du projet).
package log

import (
	"io"
	"os"
	"sync"

	"github.com/sirupsen/logrus"
)

// Fields est un alias pratique pour les champs structurés.
type Fields = logrus.Fields

var (
	mu     sync.RWMutex
	logger = newDefault()
)

// newDefault construit le logger par défaut : sortie stderr, niveau info,
// format texte lisible. Le format JSON peut être activé via UseJSON.
func newDefault() *logrus.Logger {
	l := logrus.New()
	l.SetOutput(os.Stderr)
	l.SetLevel(logrus.InfoLevel)
	l.SetFormatter(&logrus.TextFormatter{
		FullTimestamp:   true,
		TimestampFormat: "2006-01-02 15:04:05",
	})
	return l
}

// L renvoie le logger partagé.
func L() *logrus.Logger {
	mu.RLock()
	defer mu.RUnlock()
	return logger
}

// SetLevel ajuste le niveau de verbosité (ex: depuis un flag --verbose).
func SetLevel(level logrus.Level) {
	mu.Lock()
	defer mu.Unlock()
	logger.SetLevel(level)
}

// SetOutput redirige la sortie (utile pour les tests et l'audit fichier).
func SetOutput(w io.Writer) {
	mu.Lock()
	defer mu.Unlock()
	logger.SetOutput(w)
}

// UseJSON bascule le format en JSON, adapté à l'ingestion par un collecteur.
func UseJSON() {
	mu.Lock()
	defer mu.Unlock()
	logger.SetFormatter(&logrus.JSONFormatter{})
}

// WithFields démarre une entrée enrichie de champs structurés.
func WithFields(f Fields) *logrus.Entry { return L().WithFields(f) }

// Audit journalise une opération système à des fins de traçabilité.
// Chaque appel à systemd-nspawn/systemctl doit passer par ici.
func Audit(op, container string, f Fields) {
	fields := Fields{"audit": true, "op": op}
	if container != "" {
		fields["container"] = container
	}
	for k, v := range f {
		fields[k] = v
	}
	L().WithFields(fields).Info("system operation")
}
