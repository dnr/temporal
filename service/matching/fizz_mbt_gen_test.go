// Adapted from `fizz mbt-scaffold --lang go` output for
// docs/models/fairness_queue_fizzbee/mbt/queue_mbt.fizz (interfaces + action
// registry + test entry, merged into one file and moved into package
// matching so the adapter can drive the real fairTaskReader/fairTaskWriter).
//
// To run (see docs/models/fairness_queue_fizzbee/README.md):
//
//	cd docs/models/fairness_queue_fizzbee/mbt && fizz queue_mbt.fizz
//	go test ./service/matching/ -run TestFizzQueueMbt -v
//
// The test needs `fizzbee-mbt-server` and `fizzbee-mbt-runner` on PATH (or
// FIZZBEE_MBT_BIN); it skips itself otherwise. FIZZ_MBT_STATES overrides the
// default model-checker states dir.

package matching

import (
	"fmt"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"sync"
	"testing"
	"time"

	mbt "github.com/fizzbee-io/fizzbee/mbt/lib/go"
)

// mbt.RunTests reads its option-override flags assuming mbt.ParseFlags
// registered them (their un-registered zero value 0 would override our
// options to zero runs). There is no TestMain in this package, so register
// them lazily; guarded because flag.Parse must not run twice concurrently.
var fizzMbtParseFlagsOnce sync.Once

// QueueRole mirrors the Queue role in queue_mbt.fizz.
type QueueRole interface {
	mbt.Role
	ActionWriterWrite(args []mbt.Arg) (any, error)
	ActionWriterWriteOk(args []mbt.Arg) (any, error)
	ActionWriterWriteTimeout(args []mbt.Arg) (any, error)
	ActionDbReadOk(args []mbt.Arg) (any, error)
	ActionAckTask(args []mbt.Arg) (any, error)
	ActionExpireTask(args []mbt.Arg) (any, error)
}

// QueueMbtModel is the model interface for the whole specification.
type QueueMbtModel interface {
	mbt.Model
}

var fizzMbtActionRegistry = map[string]map[string]mbt.ActionFunc{
	"": {
		// pseudo-action emitted for terminal states of the graph
		"end": func(inst any, args []mbt.Arg) (any, error) { return nil, nil },
	},
	"Queue": {
		"WriterWrite": func(inst any, args []mbt.Arg) (any, error) {
			return inst.(QueueRole).ActionWriterWrite(args)
		},
		"WriterWriteOk": func(inst any, args []mbt.Arg) (any, error) {
			return inst.(QueueRole).ActionWriterWriteOk(args)
		},
		"WriterWriteTimeout": func(inst any, args []mbt.Arg) (any, error) {
			return inst.(QueueRole).ActionWriterWriteTimeout(args)
		},
		"DbReadOk": func(inst any, args []mbt.Arg) (any, error) {
			return inst.(QueueRole).ActionDbReadOk(args)
		},
		"AckTask": func(inst any, args []mbt.Arg) (any, error) {
			return inst.(QueueRole).ActionAckTask(args)
		},
		"ExpireTask": func(inst any, args []mbt.Arg) (any, error) {
			return inst.(QueueRole).ActionExpireTask(args)
		},
	},
}

func fizzMbtTestOptions() map[string]any {
	return map[string]any{
		"max-seq-runs":      2000,
		"max-parallel-runs": 0, // one shared SUT harness; sequences must not overlap
		"max-actions":       25,
	}
}

func TestFizzQueueMbt(t *testing.T) {
	statesDir := os.Getenv("FIZZ_MBT_STATES")
	if statesDir == "" {
		statesDir = filepath.Join("..", "..", "docs", "models", "fairness_queue_fizzbee", "mbt", "out", "latest")
	}
	if _, err := os.Stat(statesDir); err != nil {
		t.Skipf("fizz model-checker states not found at %s; run `fizz queue_mbt.fizz` first or set FIZZ_MBT_STATES", statesDir)
	}
	serverBin, err := exec.LookPath("fizzbee-mbt-server")
	if err != nil {
		t.Skip("fizzbee-mbt-server not on PATH")
	}
	if os.Getenv("FIZZBEE_MBT_BIN") == "" {
		if _, err := exec.LookPath("fizzbee-mbt-runner"); err != nil {
			t.Skip("fizzbee-mbt-runner not on PATH (or set FIZZBEE_MBT_BIN)")
		}
	}

	// The runner expects the trace server on localhost:50051. Reuse one that
	// is already running, otherwise spawn our own for the duration.
	if conn, err := net.DialTimeout("tcp", "localhost:50051", 100*time.Millisecond); err == nil {
		_ = conn.Close()
		t.Log("using already-running fizzbee-mbt-server")
	} else {
		cmd := exec.Command(serverBin, "--states_file", statesDir)
		// do NOT inherit our stdout/stderr: if this process dies first, an
		// orphaned server holding the pipes wedges `go test`
		logf, err := os.Create(filepath.Join(os.TempDir(), "fizzbee-mbt-server.log"))
		if err == nil {
			cmd.Stdout = logf
			cmd.Stderr = logf
			t.Cleanup(func() { _ = logf.Close() })
		}
		t.Logf("fizzbee-mbt-server log: %s", filepath.Join(os.TempDir(), "fizzbee-mbt-server.log"))
		if err := cmd.Start(); err != nil {
			t.Fatalf("failed to start fizzbee-mbt-server: %v", err)
		}
		t.Cleanup(func() { _ = cmd.Process.Kill(); _, _ = cmd.Process.Wait() })
		deadline := time.Now().Add(5 * time.Second)
		for {
			conn, err := net.DialTimeout("tcp", "localhost:50051", 100*time.Millisecond)
			if err == nil {
				_ = conn.Close()
				break
			}
			if time.Now().After(deadline) {
				t.Fatal("fizzbee-mbt-server did not start listening")
			}
			time.Sleep(50 * time.Millisecond)
		}
	}

	fizzMbtParseFlagsOnce.Do(func() { mbt.ParseFlags() })
	model := newFizzQueueMbtModel(t)
	if err := mbt.RunTests(t, model, fizzMbtActionRegistry, fizzMbtTestOptions()); err != nil {
		t.Fatal(fmt.Errorf("fizzbee mbt run failed: %w", err))
	}
}
