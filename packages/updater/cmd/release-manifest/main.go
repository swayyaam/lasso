// Command release-manifest writes latest.json, the file every Lasso release
// publishes about itself.
//
// It builds an updater.Manifest — the same type the updater reads — so the
// file a release publishes and the file the app parses have one definition,
// and a release that would not pass the updater's own validation is refused
// here, before it is published, rather than discovered by users.
//
//	go run ./packages/updater/cmd/release-manifest \
//	    -version 0.1.6 -notes notes.md -out build/latest.json \
//	    build/Lasso-app.zip build/Lasso.dmg
package main

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/swayyaam/lasso/packages/updater"
)

func main() {
	version := flag.String("version", "", "the release's version, without a leading v")
	notesPath := flag.String("notes", "", "the release notes, shown in the app before installing")
	out := flag.String("out", "", "where to write the manifest")
	flag.Parse()

	if err := run(*version, *notesPath, *out, flag.Args()); err != nil {
		fmt.Fprintln(os.Stderr, "release-manifest:", err)
		os.Exit(1)
	}
}

func run(version, notesPath, out string, assets []string) error {
	if version == "" || notesPath == "" || out == "" {
		return fmt.Errorf("-version, -notes and -out are all required")
	}
	notes, err := os.ReadFile(notesPath)
	if err != nil {
		return fmt.Errorf("reading the notes: %w", err)
	}
	if strings.TrimSpace(string(notes)) == "" {
		return fmt.Errorf("%s is empty; the app shows these notes before installing", notesPath)
	}

	m := updater.Manifest{
		SchemaVersion: updater.ManifestSchema,
		Version:       strings.TrimPrefix(version, "v"),
		Published:     time.Now().UTC().Format(time.RFC3339),
		Notes:         strings.TrimSpace(string(notes)),
		Assets:        map[string]updater.AssetInfo{},
	}
	for _, path := range assets {
		info, err := describe(path)
		if err != nil {
			return err
		}
		m.Assets[filepath.Base(path)] = info
	}

	if _, ok := m.Assets[updater.AppAsset]; !ok {
		return fmt.Errorf("no %s given; without it the release cannot be installed from inside Lasso", updater.AppAsset)
	}
	if err := m.Validate(); err != nil {
		return err
	}

	raw, err := json.MarshalIndent(m, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(out, append(raw, '\n'), 0o644)
}

func describe(path string) (updater.AssetInfo, error) {
	f, err := os.Open(path)
	if err != nil {
		return updater.AssetInfo{}, err
	}
	defer f.Close()

	h := sha256.New()
	n, err := io.Copy(h, f)
	if err != nil {
		return updater.AssetInfo{}, fmt.Errorf("reading %s: %w", path, err)
	}
	return updater.AssetInfo{Size: n, SHA256: hex.EncodeToString(h.Sum(nil))}, nil
}
