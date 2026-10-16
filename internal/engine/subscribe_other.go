//go:build !windows && !darwin && !linux

package engine

import "context"

func waitWorkerExit(context.Context, int) bool { return false }
