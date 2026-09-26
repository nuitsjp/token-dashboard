// Command release prepares signed update metadata; it does not upload anything.
package main

import (
	"crypto/ed25519"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"wailstemplate/internal/updates"
)

func main() {
	if err := run(os.Args[1:]); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
func run(args []string) error {
	if len(args) == 0 {
		return fmt.Errorf("usage: release keygen | manifest")
	}
	switch args[0] {
	case "keygen":
		fs := flag.NewFlagSet("keygen", flag.ContinueOnError)
		out := fs.String("out", "", "private-key path outside the repository")
		if err := fs.Parse(args[1:]); err != nil {
			return err
		}
		if *out == "" {
			return fmt.Errorf("-out is required")
		}
		pub, key, err := ed25519.GenerateKey(rand.Reader)
		if err != nil {
			return err
		}
		f, err := os.OpenFile(*out, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
		if err != nil {
			return err
		}
		_, writeErr := fmt.Fprintln(f, base64.StdEncoding.EncodeToString(key))
		closeErr := f.Close()
		if writeErr != nil {
			return writeErr
		}
		if closeErr != nil {
			return closeErr
		}
		fmt.Println("Public key:", base64.StdEncoding.EncodeToString(pub))
		return nil
	case "manifest":
		fs := flag.NewFlagSet("manifest", flag.ContinueOnError)
		keyfile := fs.String("key", "", "private key file")
		installer := fs.String("installer", "", "installer file")
		id := fs.String("app-id", "", "application identifier")
		ver := fs.String("version", "", "major.minor.patch")
		arch := fs.String("arch", "amd64", "amd64 or arm64")
		notes := fs.String("notes", "", "release notes")
		out := fs.String("out", "", "output directory; installer must be in it when distributed")
		if err := fs.Parse(args[1:]); err != nil {
			return err
		}
		if *keyfile == "" || *installer == "" || *id == "" || *ver == "" || *out == "" {
			return fmt.Errorf("-key, -installer, -app-id, -version and -out are required")
		}
		b, err := os.ReadFile(*keyfile)
		if err != nil {
			return err
		}
		key, err := base64.StdEncoding.DecodeString(strings.TrimSpace(string(b)))
		if err != nil {
			return err
		}
		f, err := os.Open(*installer)
		if err != nil {
			return err
		}
		h := sha256.New()
		size, err := io.Copy(h, f)
		f.Close()
		if err != nil {
			return err
		}
		data, err := updates.Sign(updates.Manifest{AppID: *id, Version: *ver, OS: "windows", Arch: *arch, Filename: filepath.Base(*installer), Size: size, SHA256: hex.EncodeToString(h.Sum(nil)), Notes: *notes}, ed25519.PrivateKey(key))
		if err != nil {
			return err
		}
		if err = os.MkdirAll(*out, 0700); err != nil {
			return err
		}
		if err = os.WriteFile(filepath.Join(*out, updates.ManifestName), data, 0644); err != nil {
			return err
		}
		fmt.Println("Created", filepath.Join(*out, updates.ManifestName))
		return nil
	default:
		return fmt.Errorf("unknown release command %q", args[0])
	}
}
