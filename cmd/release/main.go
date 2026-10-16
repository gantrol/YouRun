// Release packaging and signing tool. Never starts a GUI or execution helper.
package main

import (
	"archive/zip"
	"crypto/ed25519"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"commandmanager.local/core/internal/engine"
	"commandmanager.local/core/internal/updater"
)

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
func run() error {
	mode := flag.String("mode", "pack", "pack, manifest, inspect or keygen")
	source := flag.String("source", "", "built platform directory")
	output := flag.String("output", "", "artifact directory or keygen private-key filename")
	framework := flag.String("framework", "", "mygo or wails")
	platform := flag.String("platform", "", "windows or darwin")
	arch := flag.String("arch", "", "amd64 or arm64")
	version := flag.String("version", updater.Version, "product version")
	build := flag.Int64("build", 0, "monotonically increasing build identifier")
	keyPath := flag.String("key", "", "base64 Ed25519 private key file; omit for a local-only unsigned package")
	publicKey := flag.String("public-key", "", "base64 verification key for inspect")
	baseURL := flag.String("base-url", "", "immutable GitHub release asset base URL")
	flag.Parse()
	if *mode == "inspect" {
		release, err := updater.Inspect(*source, *publicKey)
		if err != nil {
			return err
		}
		return json.NewEncoder(os.Stdout).Encode(release)
	}
	if *mode == "keygen" {
		public, private, err := ed25519.GenerateKey(rand.Reader)
		if err != nil {
			return err
		}
		f, err := os.OpenFile(*output, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
		if err != nil {
			return err
		}
		_, err = f.WriteString(base64.StdEncoding.EncodeToString(private))
		closeErr := f.Close()
		if err != nil {
			return err
		}
		if closeErr != nil {
			return closeErr
		}
		fmt.Println(base64.StdEncoding.EncodeToString(public))
		return nil
	}
	if (*mode != "pack" && *mode != "manifest") || *source == "" || *output == "" {
		return fmt.Errorf("source and output are required")
	}
	var key ed25519.PrivateKey
	if *keyPath != "" {
		data, err := os.ReadFile(*keyPath)
		if err != nil {
			return err
		}
		decoded, err := base64.StdEncoding.DecodeString(strings.TrimSpace(string(data)))
		if err != nil || len(decoded) != ed25519.PrivateKeySize {
			return fmt.Errorf("invalid Ed25519 key")
		}
		key = decoded
	}
	if *baseURL != "" && (!strings.HasPrefix(*baseURL, "https://github.com/") || len(key) == 0) {
		return fmt.Errorf("remote feeds require an HTTPS GitHub asset URL and signing key")
	}
	release := updater.Release{Schema: 1, Product: "command-manager", Version: *version, Build: *build, Platform: *platform, Arch: *arch, Framework: *framework, ProtocolMin: engine.Protocol, ProtocolMax: engine.Protocol}
	if *framework != "mygo" && *framework != "mygo-native" && *framework != "wails" {
		return fmt.Errorf("invalid framework")
	}
	names := []string{updater.Executable(*platform, *framework), updater.CLI(*platform, *framework)}
	if *platform == "windows" {
		names = append(names, "LICENSE", "THIRD-PARTY-NOTICES.txt", "THIRD-PARTY-LICENSES.txt")
	}
	if *platform == "darwin" {
		names = nil
		bundle := filepath.Join(*source, "Command Manager "+*framework+".app")
		err := filepath.WalkDir(bundle, func(filename string, entry fs.DirEntry, err error) error {
			if err != nil {
				return err
			}
			if strings.Contains(strings.ToLower(filename), "trash") {
				return fmt.Errorf("forbidden path")
			}
			if entry.Type()&os.ModeSymlink != 0 {
				return fmt.Errorf("bundle symlinks are not supported")
			}
			if entry.IsDir() {
				return nil
			}
			if len(names) >= 256 {
				return fmt.Errorf("too many bundle files")
			}
			rel, err := filepath.Rel(*source, filename)
			if err != nil {
				return err
			}
			names = append(names, filepath.ToSlash(rel))
			return nil
		})
		if err != nil {
			return err
		}
	}
	for _, name := range names {
		if !updater.ValidPath(name) {
			return fmt.Errorf("invalid bundle path")
		}
		filename := filepath.Join(*source, filepath.FromSlash(name))
		info, err := os.Lstat(filename)
		if err != nil || !info.Mode().IsRegular() {
			return fmt.Errorf("invalid bundle file %s", name)
		}
		hash, size, err := updater.HashFile(filename)
		if err != nil {
			return err
		}
		executable := name == updater.Executable(*platform, *framework) || name == updater.CLI(*platform, *framework) || info.Mode()&0111 != 0
		release.Files = append(release.Files, updater.File{Path: name, Size: size, SHA256: hash, Executable: executable})
	}
	if err := release.Validate(); err != nil {
		return err
	}
	if err := os.MkdirAll(*output, 0700); err != nil {
		return err
	}
	metadata, err := updater.Seal(release, key)
	if err != nil {
		return err
	}
	if *mode == "manifest" {
		filename := filepath.Join(*output, "release-"+*framework+".json")
		if err := updater.WriteJSON(filename, json.RawMessage(metadata)); err != nil {
			return err
		}
		return json.NewEncoder(os.Stdout).Encode(map[string]string{"manifest": filename})
	}
	stem := fmt.Sprintf("command-manager-%s-%s-%s-%d", *platform, *arch, *framework, *build)
	filename := filepath.Join(*output, stem+".zip")
	f, err := os.Create(filename)
	if err != nil {
		return err
	}
	z := zip.NewWriter(f)
	entry, err := z.Create("update.json")
	if err == nil {
		_, err = entry.Write(metadata)
	}
	for _, file := range release.Files {
		if err != nil {
			break
		}
		header := &zip.FileHeader{Name: file.Path, Method: zip.Deflate}
		mode := os.FileMode(0644)
		if file.Executable {
			mode = 0755
		}
		header.SetMode(mode)
		var writer io.Writer
		writer, err = z.CreateHeader(header)
		if err != nil {
			break
		}
		input, openErr := os.Open(filepath.Join(*source, filepath.FromSlash(file.Path)))
		if openErr != nil {
			err = openErr
			break
		}
		_, err = io.Copy(writer, io.LimitReader(input, file.Size+1))
		input.Close()
	}
	zipErr := z.Close()
	if err == nil {
		err = zipErr
	}
	if err == nil {
		err = f.Sync()
	}
	closeErr := f.Close()
	if err == nil {
		err = closeErr
	}
	if err != nil {
		return err
	}
	hash, size, err := updater.HashFile(filename)
	if err != nil {
		return err
	}
	if size > updater.MaxArchive {
		return fmt.Errorf("update archive too large")
	}
	if *baseURL != "" {
		offer := updater.Offer{Schema: 1, Version: *version, Build: *build, URL: strings.TrimRight(*baseURL, "/") + "/" + stem + ".zip", Size: size, SHA256: hash, Platform: *platform, Arch: *arch, Framework: *framework}
		data, err := updater.Seal(offer, key)
		if err != nil {
			return err
		}
		if err = os.WriteFile(filepath.Join(*output, "update-"+*platform+"-"+*arch+"-"+*framework+".json"), data, 0600); err != nil {
			return err
		}
	}
	return json.NewEncoder(os.Stdout).Encode(map[string]any{"archive": filename, "sha256": hash, "bytes": size, "signed": len(key) > 0})
}
