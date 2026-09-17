package gitops

import (
	"context"
	"path/filepath"
	"sync"
)

// repositoryLocks serializes the operations that update one repository's
// shared refs - fetch, worktree creation and push - so parallel steps of a
// run cannot fail on each other's ref locks. Worktrees of one clone share
// its refs, so the lock is keyed by the clone's common Git directory.
var repositoryLocks sync.Map

func lockRepository(ctx context.Context, dir string) func() {
	key := dir
	if common, err := git(context.WithoutCancel(withoutLog(ctx)), dir, "rev-parse", "--path-format=absolute", "--git-common-dir"); err == nil {
		key = filepath.Clean(common)
	}
	value, _ := repositoryLocks.LoadOrStore(key, &sync.Mutex{})
	lock := value.(*sync.Mutex)
	lock.Lock()
	return lock.Unlock
}

// withoutLog keeps the lock's own lookup out of the step log.
func withoutLog(ctx context.Context) context.Context {
	return context.WithValue(ctx, logKey{}, nil)
}
