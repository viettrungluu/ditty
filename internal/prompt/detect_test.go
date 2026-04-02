package prompt

import (
	"regexp"
	"sync"
	"testing"
	"time"
)

// shortTimeout is used in tests to keep them fast.
const shortTimeout = 50 * time.Millisecond

func TestDetectsPrompt(t *testing.T) {
	var wg sync.WaitGroup
	wg.Add(1)

	detected := false
	d := NewDetector(Config{IdleTimeout: shortTimeout}, func() {
		detected = true
		wg.Done()
	})
	defer d.Stop()

	// Simulate output ending without a newline (a prompt).
	d.Feed([]byte(">>> "))
	wg.Wait()

	if !detected {
		t.Error("expected prompt to be detected")
	}
}

func TestNoDetectionOnNewline(t *testing.T) {
	detected := false
	d := NewDetector(Config{IdleTimeout: shortTimeout}, func() {
		detected = true
	})
	defer d.Stop()

	// Output ending with a newline should not trigger prompt detection.
	d.Feed([]byte("hello world\n"))
	time.Sleep(shortTimeout * 3)

	if detected {
		t.Error("did not expect prompt detection after newline-terminated output")
	}
}

func TestResetRearms(t *testing.T) {
	callCount := 0
	var mu sync.Mutex
	var wg sync.WaitGroup

	d := NewDetector(Config{IdleTimeout: shortTimeout}, func() {
		mu.Lock()
		callCount++
		mu.Unlock()
		wg.Done()
	})
	defer d.Stop()

	// First detection.
	wg.Add(1)
	d.Feed([]byte(">>> "))
	wg.Wait()

	// Reset and detect again.
	d.Reset()
	wg.Add(1)
	d.Feed([]byte("... "))
	wg.Wait()

	mu.Lock()
	if callCount != 2 {
		t.Errorf("expected 2 detections, got %d", callCount)
	}
	mu.Unlock()
}

func TestFeedResetsTimer(t *testing.T) {
	detected := false
	var wg sync.WaitGroup
	wg.Add(1)

	d := NewDetector(Config{IdleTimeout: shortTimeout}, func() {
		detected = true
		wg.Done()
	})
	defer d.Stop()

	// Feed output that ends without newline, then feed more before timeout.
	d.Feed([]byte("partial"))
	time.Sleep(shortTimeout / 3)
	d.Feed([]byte(" more"))
	time.Sleep(shortTimeout / 3)
	d.Feed([]byte(" still going\n"))

	// Now the last byte is '\n', so no prompt should fire.
	time.Sleep(shortTimeout * 3)

	if detected {
		t.Error("did not expect prompt when final output ends with newline")
	}
}

func TestStopPreventsCallback(t *testing.T) {
	detected := false
	d := NewDetector(Config{IdleTimeout: shortTimeout}, func() {
		detected = true
	})

	d.Feed([]byte(">>> "))
	d.Stop()
	time.Sleep(shortTimeout * 3)

	if detected {
		t.Error("did not expect callback after Stop")
	}
}

func TestEmptyFeedIgnored(t *testing.T) {
	detected := false
	d := NewDetector(Config{IdleTimeout: shortTimeout}, func() {
		detected = true
	})
	defer d.Stop()

	d.Feed(nil)
	d.Feed([]byte{})
	time.Sleep(shortTimeout * 3)

	if detected {
		t.Error("did not expect detection from empty feeds")
	}
}

func TestDefaultTimeout(t *testing.T) {
	d := NewDetector(Config{}, func() {})
	defer d.Stop()
	if d.cfg.IdleTimeout != DefaultIdleTimeout {
		t.Errorf("expected default timeout %v, got %v",
			DefaultIdleTimeout, d.cfg.IdleTimeout)
	}
}

func TestCallbackFiresOnlyOnce(t *testing.T) {
	callCount := 0
	var mu sync.Mutex
	var wg sync.WaitGroup
	wg.Add(1)

	d := NewDetector(Config{IdleTimeout: shortTimeout}, func() {
		mu.Lock()
		callCount++
		mu.Unlock()
		wg.Done()
	})
	defer d.Stop()

	// Feed multiple chunks without newline.
	d.Feed([]byte(">>> "))
	wg.Wait()

	// Feed more without resetting — should not fire again.
	d.Feed([]byte("extra"))
	time.Sleep(shortTimeout * 3)

	mu.Lock()
	if callCount != 1 {
		t.Errorf("expected callback to fire once, got %d", callCount)
	}
	mu.Unlock()
}

func TestRegexDetectsPrompt(t *testing.T) {
	var wg sync.WaitGroup
	wg.Add(1)

	detected := false
	d := NewDetector(Config{
		PromptRegex: regexp.MustCompile(`>>> $`),
	}, func() {
		detected = true
		wg.Done()
	})
	defer d.Stop()

	d.Feed([]byte("some output\n>>> "))
	wg.Wait()

	if !detected {
		t.Error("expected regex prompt detection")
	}
}

func TestRegexNoMatchYet(t *testing.T) {
	detected := false
	d := NewDetector(Config{
		PromptRegex: regexp.MustCompile(`>>> $`),
	}, func() {
		detected = true
	})
	defer d.Stop()

	// Output doesn't contain the prompt yet.
	d.Feed([]byte("loading...\n"))
	time.Sleep(50 * time.Millisecond)

	if detected {
		t.Error("did not expect detection without matching prompt")
	}
}

func TestRegexAcrossChunks(t *testing.T) {
	var wg sync.WaitGroup
	wg.Add(1)

	detected := false
	d := NewDetector(Config{
		PromptRegex: regexp.MustCompile(`>>> $`),
	}, func() {
		detected = true
		wg.Done()
	})
	defer d.Stop()

	// Prompt arrives across two chunks.
	d.Feed([]byte("output\n>>"))
	d.Feed([]byte("> "))
	wg.Wait()

	if !detected {
		t.Error("expected regex detection across chunks")
	}
}

func TestRegexResetClearsBuf(t *testing.T) {
	callCount := 0
	var mu sync.Mutex
	var wg sync.WaitGroup

	d := NewDetector(Config{
		PromptRegex: regexp.MustCompile(`>>> $`),
	}, func() {
		mu.Lock()
		callCount++
		mu.Unlock()
		wg.Done()
	})
	defer d.Stop()

	wg.Add(1)
	d.Feed([]byte(">>> "))
	wg.Wait()

	d.Reset()
	wg.Add(1)
	d.Feed([]byte("more\n>>> "))
	wg.Wait()

	mu.Lock()
	if callCount != 2 {
		t.Errorf("expected 2 detections after reset, got %d", callCount)
	}
	mu.Unlock()
}
