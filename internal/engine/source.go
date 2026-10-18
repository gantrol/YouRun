package engine

import (
	"bytes"
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

type cachedSource struct {
	snapshot SourceSnapshot
	at       time.Time
}

var (
	sourceCacheMu sync.Mutex
	sourceCache   = map[string]cachedSource{}
)

// InspectSource reads the Git repository status of directory without mutating
// the repository or taking Git index locks.
func InspectSource(ctx context.Context, directory, workspaceID, workspaceName string, forceFresh bool) SourceSnapshot {
	now := time.Now().UTC().Format(time.RFC3339Nano)
	trimmed := strings.TrimSpace(directory)
	if trimmed == "" || strings.Contains(strings.ToLower(trimmed), "trash") {
		return SourceSnapshot{
			WorkspaceID:   workspaceID,
			WorkspaceName: workspaceName,
			Directory:     trimmed,
			Inaccessible:  true,
			ObservedAt:    now,
		}
	}
	clean, err := filepath.Abs(trimmed)
	if err != nil {
		return SourceSnapshot{
			WorkspaceID:   workspaceID,
			WorkspaceName: workspaceName,
			Directory:     trimmed,
			Inaccessible:  true,
			ObservedAt:    now,
		}
	}
	info, err := os.Stat(clean)
	if err != nil || !info.IsDir() {
		return SourceSnapshot{
			WorkspaceID:   workspaceID,
			WorkspaceName: workspaceName,
			Directory:     clean,
			Inaccessible:  true,
			ObservedAt:    now,
		}
	}
	if !forceFresh {
		sourceCacheMu.Lock()
		if entry, ok := sourceCache[clean]; ok && time.Since(entry.at) < 1500*time.Millisecond {
			snap := entry.snapshot
			snap.WorkspaceID = workspaceID
			snap.WorkspaceName = workspaceName
			sourceCacheMu.Unlock()
			return snap
		}
		sourceCacheMu.Unlock()
	}

	snap := SourceSnapshot{
		WorkspaceID:   workspaceID,
		WorkspaceName: workspaceName,
		Directory:     clean,
		ObservedAt:    now,
	}

	gitCtx, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()

	topAndHead, err := runGitReadOnly(gitCtx, clean, "rev-parse", "--show-toplevel", "HEAD")
	if err != nil {
		// Check whether it is an unborn branch inside a valid git repository.
		topOnly, topErr := runGitReadOnly(gitCtx, clean, "rev-parse", "--show-toplevel")
		if topErr != nil {
			snap.NonGit = true
			storeSourceCache(clean, snap)
			return snap
		}
		snap.RepoRoot = filepath.Clean(strings.TrimSpace(topOnly))
		if branch, bErr := runGitReadOnly(gitCtx, clean, "symbolic-ref", "-q", "--short", "HEAD"); bErr == nil {
			snap.Branch = strings.TrimSpace(branch)
		}
		if statusOut, sErr := runGitReadOnly(gitCtx, clean, "status", "--porcelain", "--untracked-files=normal"); sErr == nil {
			snap.Dirty = strings.TrimSpace(statusOut) != ""
		}
		storeSourceCache(clean, snap)
		return snap
	}

	lines := strings.Split(strings.TrimSpace(topAndHead), "\n")
	if len(lines) >= 1 {
		snap.RepoRoot = filepath.Clean(strings.TrimSpace(lines[0]))
	}
	if len(lines) >= 2 {
		commit := strings.TrimSpace(lines[1])
		if commit != "" && commit != "HEAD" {
			snap.Commit = commit
			if len(commit) >= 7 {
				snap.ShortCommit = commit[:7]
			} else {
				snap.ShortCommit = commit
			}
		}
	}

	if branch, bErr := runGitReadOnly(gitCtx, clean, "symbolic-ref", "-q", "--short", "HEAD"); bErr == nil {
		snap.Branch = strings.TrimSpace(branch)
	} else if snap.Commit != "" {
		snap.Detached = true
	}

	if tag, tErr := runGitReadOnly(gitCtx, clean, "describe", "--tags", "--exact-match", "HEAD"); tErr == nil {
		snap.Tag = strings.TrimSpace(tag)
	}

	if statusOut, sErr := runGitReadOnly(gitCtx, clean, "status", "--porcelain", "--untracked-files=normal"); sErr == nil {
		snap.Dirty = strings.TrimSpace(statusOut) != ""
	}

	storeSourceCache(clean, snap)
	return snap
}

func storeSourceCache(clean string, snap SourceSnapshot) {
	sourceCacheMu.Lock()
	sourceCache[clean] = cachedSource{snapshot: snap, at: time.Now()}
	sourceCacheMu.Unlock()
}

func runGitReadOnly(ctx context.Context, dir string, args ...string) (string, error) {
	fullArgs := append([]string{"--no-optional-locks", "-C", dir}, args...)
	cmd := exec.CommandContext(ctx, "git", fullArgs...)
	cmd.Env = append(os.Environ(), "GIT_OPTIONAL_LOCKS=0", "GIT_TERMINAL_PROMPT=0")
	prepareHidden(cmd)
	var out bytes.Buffer
	cmd.Stdout = &out
	if err := cmd.Run(); err != nil {
		return "", err
	}
	return out.String(), nil
}
