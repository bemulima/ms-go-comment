package migrations_test

import (
	"os"
	"path/filepath"
	"reflect"
	"regexp"
	"runtime"
	"strings"
	"testing"
)

func TestInitialMigrationOwnsExactlySixTables(t *testing.T) {
	t.Parallel()

	up := readMigration(t, "001_init.up.sql")
	tablePattern := regexp.MustCompile(`(?m)^CREATE TABLE ([a-z_]+) \(`)
	matches := tablePattern.FindAllStringSubmatch(up, -1)
	got := make([]string, 0, len(matches))
	for _, match := range matches {
		got = append(got, match[1])
	}
	want := []string{
		"comment_space",
		"comment_thread",
		"comment",
		"comment_attachment",
		"comment_outbox",
		"comment_ws_ticket",
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("created tables = %v, want %v", got, want)
	}
}

func TestDownMigrationIsReverseAndNonCascading(t *testing.T) {
	t.Parallel()

	down := readMigration(t, "001_init.down.sql")
	if strings.Contains(strings.ToUpper(down), "CASCADE") {
		t.Fatal("down migration must not use CASCADE")
	}
	pattern := regexp.MustCompile(`(?m)^DROP TABLE ([a-z_]+);$`)
	matches := pattern.FindAllStringSubmatch(down, -1)
	got := make([]string, 0, len(matches))
	for _, match := range matches {
		got = append(got, match[1])
	}
	want := []string{
		"comment_ws_ticket",
		"comment_outbox",
		"comment_attachment",
		"comment",
		"comment_thread",
		"comment_space",
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("drop order = %v, want %v", got, want)
	}
}

func TestInitialMigrationContainsCriticalIntegrityRules(t *testing.T) {
	t.Parallel()

	up := readMigration(t, "001_init.up.sql")
	required := []string{
		"UNIQUE (space_id, resource_type, resource_id)",
		"UNIQUE (author_id, idempotency_key)",
		"FOREIGN KEY (thread_id, parent_id)",
		"FOREIGN KEY (thread_id, root_id)",
		"cardinality(path) = depth + 1",
		"CREATE UNIQUE INDEX uq_comment_thread_sequence",
		"ON comment USING GIN (path)",
		"FOREIGN KEY (thread_id, comment_id)",
		"octet_length(ticket_hash) = 32",
		"requested_last_sequence IS NULL OR requested_last_sequence >= 0",
		"WHERE published_at IS NULL",
	}
	for _, fragment := range required {
		if !strings.Contains(up, fragment) {
			t.Errorf("migration is missing critical rule %q", fragment)
		}
	}
}

func readMigration(t *testing.T, name string) string {
	t.Helper()
	_, source, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("cannot resolve test source path")
	}
	data, err := os.ReadFile(filepath.Join(filepath.Dir(source), "..", "..", "db", "migrations", name))
	if err != nil {
		t.Fatalf("read migration %s: %v", name, err)
	}
	return string(data)
}
