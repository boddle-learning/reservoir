package username

import (
	"context"
	"fmt"
	"sync"
	"sync/atomic"
	"testing"
)

// ---------------------------------------------------------------------------
// Mock store
// ---------------------------------------------------------------------------

// mockStore is a thread-safe in-memory SequenceStore for testing.
type mockStore struct {
	mu       sync.Mutex
	counters map[string]int
	taken    map[string]bool
}

func newMockStore() *mockStore {
	return &mockStore{
		counters: make(map[string]int),
		taken:    make(map[string]bool),
	}
}

func (m *mockStore) NextNumber(_ context.Context, base string) (int, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.counters[base]++
	return m.counters[base], nil
}

func (m *mockStore) CurrentNumber(_ context.Context, base string) (int, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.counters[base], nil
}

func (m *mockStore) IsUsernameTaken(_ context.Context, username string) (bool, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.taken[username], nil
}

func (m *mockStore) markTaken(username string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.taken[username] = true
}

// errorStore returns an error on the Nth call to NextNumber.
type errorStore struct {
	mockStore
	failOnCall int
	calls      int
}

func newErrorStore(failOn int) *errorStore {
	return &errorStore{
		mockStore:  *newMockStore(),
		failOnCall: failOn,
	}
}

func (e *errorStore) NextNumber(ctx context.Context, base string) (int, error) {
	e.mu.Lock()
	e.calls++
	call := e.calls
	e.mu.Unlock()
	if call == e.failOnCall {
		return 0, fmt.Errorf("simulated db error")
	}
	return e.mockStore.NextNumber(ctx, base)
}

// ---------------------------------------------------------------------------
// BuildBase tests
// ---------------------------------------------------------------------------

func TestBuildBase(t *testing.T) {
	tests := []struct {
		firstName string
		lastName  string
		want      string
	}{
		{"John", "Smith", "johns"},
		{"Christian", "St. John", "christians"},
		{"Mary", "O'Brien", "maryo"},
		{"Anna", "", "anna"},
		{"", "Smith", "s"},
		{"", "", ""},
		{"Jo-Anne", "Lee", "joannel"},
		{"ABCDEFGHIJKLMNOPQRSTuvwxyz", "Z", "abcdefghijklmnopqrstuvwxyzz"},
		{"Chris", "3rd", "chrisr"},
		{"ALLCAPS", "LAST", "allcapsl"},
		{"MiXeD", "CaSe", "mixedc"},
		{"a", "b", "ab"},
		{"  spaces  ", "  tab  ", "spacest"},
		{"123", "456", ""},          // no letters at all
		{"abc123", "def", "abcd"},   // digits stripped from first
		{"first", "  ", "first"},    // whitespace-only last name
		{"José", "García", "joség"}, // accented characters preserved
	}

	for _, tt := range tests {
		t.Run(tt.firstName+"_"+tt.lastName, func(t *testing.T) {
			got := BuildBase(tt.firstName, tt.lastName)
			if got != tt.want {
				t.Errorf("BuildBase(%q, %q) = %q, want %q", tt.firstName, tt.lastName, got, tt.want)
			}
		})
	}
}

func TestBuildBase_LongNameNotTruncated(t *testing.T) {
	base := BuildBase("ChristianJames", "Smith")
	if base != "christianjamess" {
		t.Errorf("got %q, want %q", base, "christianjamess")
	}
	if len(base) != 15 {
		t.Errorf("len = %d, want 15", len(base))
	}
}

// ---------------------------------------------------------------------------
// formatUsername tests
// ---------------------------------------------------------------------------

func TestFormatUsername(t *testing.T) {
	tests := []struct {
		base string
		num  int
		want string
	}{
		{"christians", 1, "christians1"},
		{"christians", 9, "christians9"},
		{"christians", 10, "christians10"},
		{"christians", 999, "christians999"},
		{"christianjames", 1, "christianjames1"},
		{"christianjames", 9, "christianjames9"},
		{"christianjames", 10, "christianjame10"},
		{"christianjames", 100, "christianjam100"},
		{"a", 1, "a1"},
		{"a", 99999999999999, "a99999999999999"},
		{"ab", 1, "ab1"},
	}

	for _, tt := range tests {
		t.Run(tt.want, func(t *testing.T) {
			got := formatUsername(tt.base, tt.num)
			if got != tt.want {
				t.Errorf("formatUsername(%q, %d) = %q, want %q", tt.base, tt.num, got, tt.want)
			}
			if len(got) > MaxUsernameLength {
				t.Errorf("username %q exceeds max length %d", got, MaxUsernameLength)
			}
		})
	}
}

func TestFormatUsername_TooLargeNumber(t *testing.T) {
	// A 15+ digit number leaves no room for even a 1-char base.
	got := formatUsername("a", 1000000000000000)
	if got != "" {
		t.Errorf("expected empty string for oversized number, got %q", got)
	}
}

// ---------------------------------------------------------------------------
// Generate — basic behaviour
// ---------------------------------------------------------------------------

func TestGenerate_FirstUsername(t *testing.T) {
	store := newMockStore()
	svc := NewService(store)

	got, err := svc.Generate(context.Background(), "John", "Smith")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != "johns1" {
		t.Errorf("got %q, want %q", got, "johns1")
	}
}

func TestGenerate_Sequential(t *testing.T) {
	store := newMockStore()
	svc := NewService(store)
	ctx := context.Background()

	var results []string
	for i := 0; i < 5; i++ {
		name, err := svc.Generate(ctx, "John", "Smith")
		if err != nil {
			t.Fatalf("call %d: unexpected error: %v", i, err)
		}
		results = append(results, name)
	}

	expected := []string{"johns1", "johns2", "johns3", "johns4", "johns5"}
	for i, want := range expected {
		if results[i] != want {
			t.Errorf("call %d: got %q, want %q", i, results[i], want)
		}
	}
}

func TestGenerate_DifferentNames(t *testing.T) {
	store := newMockStore()
	svc := NewService(store)
	ctx := context.Background()

	a, _ := svc.Generate(ctx, "John", "Smith")
	b, _ := svc.Generate(ctx, "Jane", "Doe")
	c, _ := svc.Generate(ctx, "John", "Smith")

	if a != "johns1" {
		t.Errorf("a: got %q, want johns1", a)
	}
	if b != "janed1" {
		t.Errorf("b: got %q, want janed1", b)
	}
	if c != "johns2" {
		t.Errorf("c: got %q, want johns2", c)
	}
}

func TestGenerate_EmptyFirstName(t *testing.T) {
	svc := NewService(newMockStore())
	got, err := svc.Generate(context.Background(), "", "Smith")
	// "s" is a valid base (just the last initial), so this should succeed.
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != "s1" {
		t.Errorf("got %q, want %q", got, "s1")
	}
}

func TestGenerate_EmptyBoth(t *testing.T) {
	svc := NewService(newMockStore())
	_, err := svc.Generate(context.Background(), "", "")
	if err == nil {
		t.Error("expected error for empty first and last name")
	}
}

func TestGenerate_LongName_Truncation(t *testing.T) {
	store := newMockStore()
	svc := NewService(store)
	ctx := context.Background()

	name, err := svc.Generate(ctx, "ChristianJames", "Smith")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// base "christianjamess" (15 chars) truncated to 14 → "christianjames"
	// number 1 → "christianjames1" (15 chars)
	if name != "christianjames1" {
		t.Errorf("got %q, want %q", name, "christianjames1")
	}
	if len(name) > MaxUsernameLength {
		t.Errorf("len %d exceeds max %d", len(name), MaxUsernameLength)
	}
}

func TestGenerate_LongName_MultiDigitTruncation(t *testing.T) {
	store := newMockStore()
	svc := NewService(store)
	ctx := context.Background()

	// Burn through 9 numbers so the next is 10 (2 digits).
	for i := 0; i < 9; i++ {
		svc.Generate(ctx, "ChristianJames", "Smith")
	}

	name, err := svc.Generate(ctx, "ChristianJames", "Smith")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// base "christianjames" (14 chars), number 10 → truncate to 13 + "10" = 15
	if name != "christianjame10" {
		t.Errorf("got %q, want %q", name, "christianjame10")
	}
	if len(name) > MaxUsernameLength {
		t.Errorf("len %d exceeds max %d", len(name), MaxUsernameLength)
	}
}

// ---------------------------------------------------------------------------
// Generate — retry on collision
// ---------------------------------------------------------------------------

func TestGenerate_RetriesOnCollision(t *testing.T) {
	store := newMockStore()
	// Pre-mark "johns1" as taken in the students table.
	store.markTaken("johns1")

	svc := NewService(store)
	got, err := svc.Generate(context.Background(), "John", "Smith")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// Should skip johns1 and return johns2.
	if got != "johns2" {
		t.Errorf("got %q, want %q", got, "johns2")
	}
}

func TestGenerate_RetriesMultipleCollisions(t *testing.T) {
	store := newMockStore()
	// Mark several as taken.
	store.markTaken("johns1")
	store.markTaken("johns2")
	store.markTaken("johns3")

	svc := NewService(store)
	got, err := svc.Generate(context.Background(), "John", "Smith")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != "johns4" {
		t.Errorf("got %q, want %q", got, "johns4")
	}
}

func TestGenerate_ExhaustsRetries(t *testing.T) {
	store := newMockStore()
	// Mark more than maxRetries usernames as taken.
	for i := 1; i <= maxRetries+5; i++ {
		store.markTaken(fmt.Sprintf("johns%d", i))
	}

	svc := NewService(store)
	_, err := svc.Generate(context.Background(), "John", "Smith")
	if err == nil {
		t.Error("expected error after exhausting retries")
	}
}

// ---------------------------------------------------------------------------
// Generate — error propagation
// ---------------------------------------------------------------------------

func TestGenerate_NextNumberError(t *testing.T) {
	store := newErrorStore(1) // fail on first call
	svc := NewService(store)

	_, err := svc.Generate(context.Background(), "John", "Smith")
	if err == nil {
		t.Error("expected error from NextNumber failure")
	}
}

func TestGenerate_NextNumberErrorOnRetry(t *testing.T) {
	store := newErrorStore(2) // succeed first, fail on retry
	store.markTaken("johns1") // force a retry

	svc := NewService(store)
	_, err := svc.Generate(context.Background(), "John", "Smith")
	if err == nil {
		t.Error("expected error from NextNumber failure on retry")
	}
}

// ---------------------------------------------------------------------------
// Concurrency tests
// ---------------------------------------------------------------------------

func TestGenerate_ConcurrentSameBase(t *testing.T) {
	store := newMockStore()
	svc := NewService(store)
	ctx := context.Background()

	const goroutines = 100
	results := make([]string, goroutines)
	var wg sync.WaitGroup
	wg.Add(goroutines)

	for i := 0; i < goroutines; i++ {
		go func(idx int) {
			defer wg.Done()
			name, err := svc.Generate(ctx, "John", "Smith")
			if err != nil {
				t.Errorf("goroutine %d: unexpected error: %v", idx, err)
				return
			}
			results[idx] = name
		}(i)
	}
	wg.Wait()

	// Every generated username must be unique.
	seen := make(map[string]int)
	for i, name := range results {
		if name == "" {
			continue // errored goroutine
		}
		if prev, ok := seen[name]; ok {
			t.Errorf("duplicate username %q generated by goroutines %d and %d", name, prev, i)
		}
		seen[name] = i
	}

	if len(seen) != goroutines {
		t.Errorf("expected %d unique usernames, got %d", goroutines, len(seen))
	}
}

func TestGenerate_ConcurrentDifferentBases(t *testing.T) {
	store := newMockStore()
	svc := NewService(store)
	ctx := context.Background()

	names := []struct{ first, last string }{
		{"John", "Smith"},
		{"Jane", "Doe"},
		{"Alex", "Brown"},
		{"Sam", "Wilson"},
	}

	const perName = 50
	total := len(names) * perName
	results := make([]string, total)
	var wg sync.WaitGroup
	wg.Add(total)

	for i, n := range names {
		for j := 0; j < perName; j++ {
			go func(idx int, first, last string) {
				defer wg.Done()
				name, err := svc.Generate(ctx, first, last)
				if err != nil {
					t.Errorf("goroutine %d: unexpected error: %v", idx, err)
					return
				}
				results[idx] = name
			}(i*perName+j, n.first, n.last)
		}
	}
	wg.Wait()

	seen := make(map[string]int)
	for i, name := range results {
		if name == "" {
			continue
		}
		if prev, ok := seen[name]; ok {
			t.Errorf("duplicate username %q from goroutines %d and %d", name, prev, i)
		}
		seen[name] = i
	}

	if len(seen) != total {
		t.Errorf("expected %d unique usernames, got %d", total, len(seen))
	}
}

func TestGenerate_ConcurrentWithCollisions(t *testing.T) {
	store := newMockStore()
	// Pre-mark odd numbers as taken so every other attempt must retry.
	for i := 1; i <= 200; i += 2 {
		store.markTaken(fmt.Sprintf("johns%d", i))
	}

	svc := NewService(store)
	ctx := context.Background()

	const goroutines = 50
	results := make([]string, goroutines)
	var wg sync.WaitGroup
	var errCount atomic.Int32
	wg.Add(goroutines)

	for i := 0; i < goroutines; i++ {
		go func(idx int) {
			defer wg.Done()
			name, err := svc.Generate(ctx, "John", "Smith")
			if err != nil {
				errCount.Add(1)
				return
			}
			results[idx] = name
		}(i)
	}
	wg.Wait()

	seen := make(map[string]bool)
	for _, name := range results {
		if name == "" {
			continue
		}
		if seen[name] {
			t.Errorf("duplicate username %q under concurrent collision retry", name)
		}
		seen[name] = true
	}

	successCount := goroutines - int(errCount.Load())
	if successCount == 0 {
		t.Error("all goroutines failed, expected some successes")
	}
	if len(seen) != successCount {
		t.Errorf("unique count %d != success count %d", len(seen), successCount)
	}
}

// ---------------------------------------------------------------------------
// Edge cases
// ---------------------------------------------------------------------------

func TestGenerate_SingleCharName(t *testing.T) {
	svc := NewService(newMockStore())
	got, err := svc.Generate(context.Background(), "A", "B")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != "ab1" {
		t.Errorf("got %q, want %q", got, "ab1")
	}
}

func TestGenerate_UnicodeNames(t *testing.T) {
	svc := NewService(newMockStore())
	got, err := svc.Generate(context.Background(), "José", "García")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// "josé" + "g" = "joség", then + "1"
	if got != "joség1" {
		t.Errorf("got %q, want %q", got, "joség1")
	}
}

func TestGenerate_SpecialCharsStripped(t *testing.T) {
	svc := NewService(newMockStore())
	got, err := svc.Generate(context.Background(), "Mary-Jane", "O'Connor")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != "maryjaneo1" {
		t.Errorf("got %q, want %q", got, "maryjaneo1")
	}
}

func TestGenerate_OnlyDigitsInName(t *testing.T) {
	svc := NewService(newMockStore())
	_, err := svc.Generate(context.Background(), "123", "456")
	if err == nil {
		t.Error("expected error for names with no letters")
	}
}

func TestGenerate_WhitespaceOnlyLastName(t *testing.T) {
	svc := NewService(newMockStore())
	got, err := svc.Generate(context.Background(), "Alex", "   ")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// No last initial extracted.
	if got != "alex1" {
		t.Errorf("got %q, want %q", got, "alex1")
	}
}

func TestGenerate_MaxLengthAlwaysRespected(t *testing.T) {
	store := newMockStore()
	svc := NewService(store)
	ctx := context.Background()

	// Generate 200 usernames for a long name and verify all fit.
	for i := 0; i < 200; i++ {
		name, err := svc.Generate(ctx, "ChristianJames", "Smith")
		if err != nil {
			t.Fatalf("iteration %d: unexpected error: %v", i, err)
		}
		if len(name) > MaxUsernameLength {
			t.Errorf("iteration %d: username %q length %d exceeds max %d", i, name, len(name), MaxUsernameLength)
		}
		if name == "" {
			t.Errorf("iteration %d: got empty username", i)
		}
	}
}
