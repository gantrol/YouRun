package projectmeta

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

var avatarName = regexp.MustCompile(`^[a-f0-9]{64}\.(png|svg)$`)

func validAvatar(name string) bool { return avatarName.MatchString(name) }

func AvatarPath(directory, name string) string {
	if !validAvatar(name) {
		return ""
	}
	return filepath.Join(directory, "project-avatars", name)
}

// SaveAvatar only accepts normalized thumbnail bytes. The content file is
// committed before the metadata pointer under the same cross-process lock.
func SaveAvatar(ctx context.Context, directory, id, extension string, data []byte, revision int64) (Entry, error) {
	if (extension != ".png" && extension != ".svg") || len(data) == 0 || len(data) > 128<<10 {
		return Entry{}, errors.New("invalidRequest")
	}
	digest := sha256.Sum256(data)
	name := hex.EncodeToString(digest[:]) + extension
	return update(ctx, directory, id, "", nil, name, data, revision)
}

func ReadAvatar(directory, name string) ([]byte, error) {
	if !validAvatar(name) {
		return nil, errors.New("invalid avatar")
	}
	root, err := os.OpenRoot(filepath.Join(directory, "project-avatars"))
	if err != nil {
		return nil, err
	}
	defer root.Close()
	info, err := root.Lstat(name)
	if err != nil || !info.Mode().IsRegular() || info.Size() > 128<<10 {
		return nil, errors.New("invalid avatar")
	}
	f, err := root.Open(name)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	data, err := io.ReadAll(io.LimitReader(f, (128<<10)+1))
	digest := sha256.Sum256(data)
	if err != nil || len(data) > 128<<10 || hex.EncodeToString(digest[:]) != strings.TrimSuffix(name, filepath.Ext(name)) {
		return nil, errors.New("invalid avatar")
	}
	return data, nil
}

func writeAvatar(directory, name string, data []byte) error {
	if !validAvatar(name) {
		return errors.New("invalid avatar")
	}
	if existing, err := ReadAvatar(directory, name); err == nil && bytes.Equal(existing, data) {
		return nil
	}
	folder := filepath.Join(directory, "project-avatars")
	if err := os.MkdirAll(folder, 0700); err != nil {
		return err
	}
	f, err := os.CreateTemp(folder, ".avatar-*")
	if err != nil {
		return err
	}
	temporary := f.Name()
	defer os.Remove(temporary)
	if _, err = f.Write(data); err == nil {
		err = f.Sync()
	}
	closeErr := f.Close()
	if err != nil {
		return err
	}
	if closeErr != nil {
		return closeErr
	}
	return os.Rename(temporary, AvatarPath(directory, name))
}

// PrepareAvatar stages immutable normalized bytes without changing any project's
// saved metadata. Canceling the editor leaves the previous avatar reference intact.
func PrepareAvatar(directory, extension string, data []byte) (string, error) {
	if (extension != ".png" && extension != ".svg") || len(data) == 0 || len(data) > 128<<10 {
		return "", errors.New("invalidRequest")
	}
	digest := sha256.Sum256(data)
	name := hex.EncodeToString(digest[:]) + extension
	return name, writeAvatar(directory, name, data)
}
