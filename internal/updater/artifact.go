// Package updater stages immutable application versions; it never manages command processes.
package updater

import (
	"archive/zip"
	"context"
	"crypto/ed25519"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"os"
	"path"
	"path/filepath"
	"runtime"
	"strings"
)

var Version = "0.1.1"
var Build = "0"

// Distribution is set by the build script; MSIX versions are serviced by Windows.
var Distribution = "portable"

func StoreManaged() bool { return Distribution == "msix" }

func Development() bool { return Build == "0" && !StoreManaged() }

const DefaultRepository = "gantrol/YouRun"
const DefaultPublicKey = "ZvRE6gn+ruJUagXIYZf7Ur3wi72/xlZIp6CgYDj7JJY="

const MaxArchive = 256 << 20
const MaxExpanded = 512 << 20
const MaxMetadata = 1 << 20

type File struct {
	Path       string `json:"path"`
	Size       int64  `json:"size"`
	SHA256     string `json:"sha256"`
	Executable bool   `json:"executable"`
}
type Release struct {
	Schema      int    `json:"schema"`
	Product     string `json:"product"`
	Version     string `json:"version"`
	Build       int64  `json:"build"`
	Platform    string `json:"platform"`
	Arch        string `json:"arch"`
	Framework   string `json:"framework"`
	ProtocolMin int    `json:"protocolMin"`
	ProtocolMax int    `json:"protocolMax"`
	Files       []File `json:"files"`
}
type Envelope struct {
	Payload   json.RawMessage `json:"payload"`
	Signature string          `json:"signature"`
}
type Offer struct {
	Schema    int    `json:"schema"`
	Version   string `json:"version"`
	Build     int64  `json:"build"`
	URL       string `json:"url"`
	Size      int64  `json:"size"`
	SHA256    string `json:"sha256"`
	Platform  string `json:"platform"`
	Arch      string `json:"arch"`
	Framework string `json:"framework"`
}
type Staged struct {
	Directory string  `json:"directory"`
	Release   Release `json:"release"`
}

func Seal(value any, key ed25519.PrivateKey) ([]byte, error) {
	payload, err := json.Marshal(value)
	if err != nil {
		return nil, err
	}
	envelope := Envelope{Payload: payload}
	if len(key) == ed25519.PrivateKeySize {
		envelope.Signature = base64.StdEncoding.EncodeToString(ed25519.Sign(key, payload))
	}
	return json.Marshal(envelope)
}
func OpenEnvelope(data []byte, publicKey string, value any) error {
	var envelope Envelope
	if len(data) > MaxMetadata || json.Unmarshal(data, &envelope) != nil {
		return errors.New("updateInvalidPackage")
	}
	if publicKey != "" {
		key, err := base64.StdEncoding.DecodeString(publicKey)
		sig, sigErr := base64.StdEncoding.DecodeString(envelope.Signature)
		if err != nil || sigErr != nil || len(key) != ed25519.PublicKeySize || !ed25519.Verify(key, envelope.Payload, sig) {
			return errors.New("updateSignatureFailed")
		}
	}
	if json.Unmarshal(envelope.Payload, value) != nil {
		return errors.New("updateInvalidPackage")
	}
	return nil
}
func HashFile(filename string) (string, int64, error) {
	f, err := os.Open(filename)
	if err != nil {
		return "", 0, err
	}
	defer f.Close()
	h := sha256.New()
	n, err := io.Copy(h, io.LimitReader(f, MaxExpanded+1))
	if n > MaxExpanded {
		return "", n, errors.New("updateTooLarge")
	}
	return hex.EncodeToString(h.Sum(nil)), n, err
}
func ValidPath(name string) bool {
	return name != "" && name != "." && !strings.ContainsAny(name, "\\:\x00") && !strings.Contains(strings.ToLower(name), "trash") && path.Clean(name) == name && !strings.HasPrefix(name, "/") && filepath.IsLocal(filepath.FromSlash(name))
}
func Executable(platform, framework string) string {
	name := "command-manager-" + framework
	if platform == "windows" {
		return "YouRun.exe"
	}
	return "Command Manager " + framework + ".app/Contents/MacOS/" + name
}
func CLI(platform, framework string) string {
	if platform == "windows" {
		return "yourun.cmd"
	}
	return "Command Manager " + framework + ".app/Contents/MacOS/yourun"
}

// Executable resolves the entry declared by the release, including versions
// staged before Windows builds adopted the public product name.
func (r Release) Executable() string {
	name := Executable(r.Platform, r.Framework)
	for _, file := range r.Files {
		if file.Path == name {
			return name
		}
	}
	if r.Platform == "windows" {
		legacy := "command-manager-" + r.Framework + ".exe"
		for _, file := range r.Files {
			if file.Path == legacy {
				return legacy
			}
		}
	}
	return name
}

func (r Release) Validate() error {
	if r.Schema != 1 || r.Product != "command-manager" || r.Build <= 0 || len(r.Version) > 64 || r.Version == "" || (r.Platform != "windows" && r.Platform != "darwin") || (r.Arch != "amd64" && r.Arch != "arm64") || (r.Framework != "wails" && r.Framework != "mygo" && r.Framework != "mygo-native") || r.ProtocolMin <= 0 || r.ProtocolMax < r.ProtocolMin || len(r.Files) < 2 || len(r.Files) > 256 {
		return errors.New("updateInvalidPackage")
	}
	seen := map[string]bool{}
	var total int64
	for _, file := range r.Files {
		key := strings.ToLower(file.Path)
		hash, err := hex.DecodeString(file.SHA256)
		if !ValidPath(file.Path) || seen[key] || key == "update.json" || key == "release.json" || err != nil || len(hash) != sha256.Size || file.Size < 0 || file.Size > MaxExpanded {
			return errors.New("updateInvalidPackage")
		}
		seen[key] = true
		total += file.Size
		if total > MaxExpanded {
			return errors.New("updateTooLarge")
		}
	}
	executable := r.Executable()
	for _, required := range []string{executable, CLI(r.Platform, r.Framework)} {
		found := false
		for _, f := range r.Files {
			// Accept earlier framework-named scripts and standalone CLIs.
			// A standalone yourun.exe cannot coexist with YouRun.exe on Windows.
			legacyCLI := false
			if r.Platform == "windows" && required == CLI(r.Platform, r.Framework) {
				legacyCLI = f.Path == "yourun-"+r.Framework+".cmd" ||
					(executable != Executable(r.Platform, r.Framework) && f.Path == "yourun.exe")
			}
			if (f.Path == required || legacyCLI) && f.Executable {
				found = true
			}
		}
		if !found {
			return errors.New("updateInvalidPackage")
		}
	}
	return nil
}
func Inspect(archive, key string) (Release, error) {
	z, err := zip.OpenReader(archive)
	if err != nil {
		return Release{}, errors.New("updateInvalidPackage")
	}
	defer z.Close()
	if len(z.File) > 512 {
		return Release{}, errors.New("updateTooLarge")
	}
	var data []byte
	seen := map[string]bool{}
	for _, entry := range z.File {
		name := strings.TrimSuffix(entry.Name, "/")
		if !ValidPath(name) || seen[strings.ToLower(name)] || entry.Mode()&os.ModeSymlink != 0 {
			return Release{}, errors.New("updateInvalidPackage")
		}
		seen[strings.ToLower(name)] = true
		if entry.Name != "update.json" {
			continue
		}
		f, e := entry.Open()
		if e != nil {
			return Release{}, e
		}
		data, err = io.ReadAll(io.LimitReader(f, MaxMetadata+1))
		f.Close()
		if err != nil {
			return Release{}, err
		}
	}
	var release Release
	if err := OpenEnvelope(data, key, &release); err != nil {
		return release, err
	}
	return release, release.Validate()
}
func Stage(ctx context.Context, archive, root, key, framework string, protocol int) (*Staged, error) {
	release, err := Inspect(archive, key)
	if err != nil {
		return nil, err
	}
	z, err := zip.OpenReader(archive)
	if err != nil {
		return nil, err
	}
	defer z.Close()
	files := map[string]*zip.File{}
	for _, f := range z.File {
		if f.FileInfo().IsDir() || f.Name == "update.json" {
			continue
		}
		files[f.Name] = f
	}
	if len(files) != len(release.Files) {
		return nil, errors.New("updateInvalidPackage")
	}
	return stageFiles(ctx, root, release, framework, protocol, func(file File) (io.ReadCloser, error) {
		entry := files[file.Path]
		if entry == nil || entry.UncompressedSize64 != uint64(file.Size) {
			return nil, errors.New("updateInvalidPackage")
		}
		return entry.Open()
	})
}

// StageLocal accepts only output from an explicitly trusted source checkout.
// Downloaded and manually selected packages must continue through Stage.
func StageLocal(ctx context.Context, source, root, framework string, protocol int, build int64) (*Staged, error) {
	if framework != "wails" && framework != "mygo" && framework != "mygo-native" {
		return nil, errors.New("updateInvalidPackage")
	}
	directory, err := os.OpenRoot(source)
	if err != nil {
		return nil, err
	}
	defer directory.Close()
	manifest, err := directory.Open("release-" + framework + ".json")
	if err != nil {
		return nil, err
	}
	data, err := io.ReadAll(io.LimitReader(manifest, MaxMetadata+1))
	manifest.Close()
	if err != nil {
		return nil, err
	}
	var release Release
	if err := OpenEnvelope(data, "", &release); err != nil {
		return nil, err
	}
	if release.Build != build {
		return nil, errors.New("updateInvalidPackage")
	}
	return stageFiles(ctx, root, release, framework, protocol, func(file File) (io.ReadCloser, error) {
		name := filepath.FromSlash(file.Path)
		info, err := directory.Lstat(name)
		if err != nil || !info.Mode().IsRegular() || info.Size() != file.Size {
			return nil, errors.New("updateInvalidPackage")
		}
		return directory.Open(name)
	})
}

func stageFiles(ctx context.Context, root string, release Release, framework string, protocol int, open func(File) (io.ReadCloser, error)) (*Staged, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if err := release.Validate(); err != nil {
		return nil, err
	}
	if release.Platform != runtime.GOOS || release.Arch != runtime.GOARCH || release.Framework != framework {
		return nil, errors.New("updateWrongPlatform")
	}
	if protocol < release.ProtocolMin || protocol > release.ProtocolMax {
		return nil, errors.New("updateIncompatible")
	}
	if err := os.MkdirAll(root, 0700); err != nil {
		return nil, err
	}
	directory, err := os.MkdirTemp(root, "release-")
	if err != nil {
		return nil, err
	}
	complete := false
	defer func() {
		if !complete {
			RemoveManaged(root, directory)
		}
	}()
	for _, file := range release.Files {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		target := filepath.Join(directory, filepath.FromSlash(file.Path))
		if err = os.MkdirAll(filepath.Dir(target), 0700); err != nil {
			return nil, err
		}
		input, err := open(file)
		if err != nil {
			return nil, err
		}
		mode := os.FileMode(0600)
		if file.Executable {
			mode = 0700
		}
		output, err := os.OpenFile(target, os.O_CREATE|os.O_EXCL|os.O_WRONLY, mode)
		if err != nil {
			input.Close()
			return nil, err
		}
		hash := sha256.New()
		n, copyErr := io.Copy(io.MultiWriter(output, hash), &contextReader{ctx: ctx, reader: io.LimitReader(input, file.Size+1)})
		input.Close()
		syncErr := output.Sync()
		closeErr := output.Close()
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		if copyErr != nil || syncErr != nil || closeErr != nil || n != file.Size || hex.EncodeToString(hash.Sum(nil)) != file.SHA256 {
			return nil, errors.New("updateHashFailed")
		}
	}
	// Metadata is outside a signed .app bundle.
	staged := &Staged{Directory: directory, Release: release}
	if err = WriteJSON(filepath.Join(directory, "release.json"), staged); err != nil {
		return nil, err
	}
	complete = true
	return staged, nil
}

type contextReader struct {
	ctx    context.Context
	reader io.Reader
}

func (r *contextReader) Read(p []byte) (int, error) {
	if err := r.ctx.Err(); err != nil {
		return 0, err
	}
	return r.reader.Read(p)
}
func Verify(staged *Staged) error {
	if err := staged.Release.Validate(); err != nil {
		return err
	}
	root, err := os.OpenRoot(staged.Directory)
	if err != nil {
		return err
	}
	defer root.Close()
	for _, file := range staged.Release.Files {
		info, err := root.Lstat(filepath.FromSlash(file.Path))
		if err != nil || !info.Mode().IsRegular() || info.Size() != file.Size {
			return errors.New("updateHashFailed")
		}
		f, err := root.Open(filepath.FromSlash(file.Path))
		if err != nil {
			return err
		}
		h := sha256.New()
		n, e := io.Copy(h, io.LimitReader(f, file.Size+1))
		f.Close()
		if e != nil || n != file.Size || hex.EncodeToString(h.Sum(nil)) != file.SHA256 {
			return errors.New("updateHashFailed")
		}
	}
	return nil
}

// Only direct, non-symlink release directories owned by this updater may be removed.
func RemoveManaged(root, directory string) error {
	absRoot, err := filepath.Abs(root)
	if err != nil {
		return err
	}
	absDir, err := filepath.Abs(directory)
	if err != nil {
		return err
	}
	if filepath.Dir(absDir) != absRoot || !strings.HasPrefix(filepath.Base(absDir), "release-") {
		return errors.New("updateInvalidPackage")
	}
	info, err := os.Lstat(absDir)
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil || !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
		return errors.New("updateInvalidPackage")
	}
	resolvedRoot, err := filepath.EvalSymlinks(absRoot)
	if err != nil {
		return err
	}
	resolvedDir, err := filepath.EvalSymlinks(absDir)
	if err != nil || filepath.Dir(resolvedDir) != resolvedRoot {
		return errors.New("updateInvalidPackage")
	}
	return os.RemoveAll(absDir)
}
