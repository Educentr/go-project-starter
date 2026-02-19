package config

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestParseQueueSpec_Valid(t *testing.T) {
	content := `
queues:
  - id: 1
    name: emails
    fields:
      - name: to
        type: string
      - name: body
        type: "[]byte"
  - id: 2
    name: tasks
    fields:
      - name: data
        type: "[]byte"
`
	dir := t.TempDir()
	path := filepath.Join(dir, "queues.yaml")
	require.NoError(t, os.WriteFile(path, []byte(content), 0644))

	spec, err := ParseQueueSpec(path)
	require.NoError(t, err)
	require.Len(t, spec.Queues, 2)
	assert.Equal(t, 1, spec.Queues[0].ID)
	assert.Equal(t, "emails", spec.Queues[0].Name)
	assert.Len(t, spec.Queues[0].Fields, 2)
	assert.Equal(t, "to", spec.Queues[0].Fields[0].Name)
	assert.Equal(t, "string", spec.Queues[0].Fields[0].Type)
}

func TestParseQueueSpec_NoQueues(t *testing.T) {
	content := `queues: []`
	dir := t.TempDir()
	path := filepath.Join(dir, "queues.yaml")
	require.NoError(t, os.WriteFile(path, []byte(content), 0644))

	_, err := ParseQueueSpec(path)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "no queues defined")
}

func TestParseQueueSpec_DuplicateID(t *testing.T) {
	content := `
queues:
  - id: 1
    name: a
    fields:
      - name: x
        type: string
  - id: 1
    name: b
    fields:
      - name: y
        type: string
`
	dir := t.TempDir()
	path := filepath.Join(dir, "queues.yaml")
	require.NoError(t, os.WriteFile(path, []byte(content), 0644))

	_, err := ParseQueueSpec(path)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "duplicate queue id: 1")
}

func TestParseQueueSpec_DuplicateName(t *testing.T) {
	content := `
queues:
  - id: 1
    name: same
    fields:
      - name: x
        type: string
  - id: 2
    name: same
    fields:
      - name: y
        type: string
`
	dir := t.TempDir()
	path := filepath.Join(dir, "queues.yaml")
	require.NoError(t, os.WriteFile(path, []byte(content), 0644))

	_, err := ParseQueueSpec(path)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "duplicate queue name: same")
}

func TestParseQueueSpec_NoFields(t *testing.T) {
	content := `
queues:
  - id: 1
    name: empty
    fields: []
`
	dir := t.TempDir()
	path := filepath.Join(dir, "queues.yaml")
	require.NoError(t, os.WriteFile(path, []byte(content), 0644))

	_, err := ParseQueueSpec(path)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "has no fields")
}

func TestParseQueueSpec_InvalidType(t *testing.T) {
	content := `
queues:
  - id: 1
    name: test
    fields:
      - name: x
        type: float64
`
	dir := t.TempDir()
	path := filepath.Join(dir, "queues.yaml")
	require.NoError(t, os.WriteFile(path, []byte(content), 0644))

	_, err := ParseQueueSpec(path)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "invalid type")
}

func TestParseQueueSpec_DuplicateFieldName(t *testing.T) {
	content := `
queues:
  - id: 1
    name: test
    fields:
      - name: x
        type: string
      - name: x
        type: int
`
	dir := t.TempDir()
	path := filepath.Join(dir, "queues.yaml")
	require.NoError(t, os.WriteFile(path, []byte(content), 0644))

	_, err := ParseQueueSpec(path)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "duplicate field name: x")
}

func TestParseQueueSpec_AllTypes(t *testing.T) {
	content := `
queues:
  - id: 1
    name: all_types
    fields:
      - name: f_int
        type: int
      - name: f_int64
        type: int64
      - name: f_string
        type: string
      - name: f_bool
        type: bool
      - name: f_bytes
        type: "[]byte"
      - name: f_ints
        type: "[]int"
      - name: f_int64s
        type: "[]int64"
`
	dir := t.TempDir()
	path := filepath.Join(dir, "queues.yaml")
	require.NoError(t, os.WriteFile(path, []byte(content), 0644))

	spec, err := ParseQueueSpec(path)
	require.NoError(t, err)
	assert.Len(t, spec.Queues[0].Fields, 7)
}
