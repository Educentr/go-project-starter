package config

import (
	"fmt"
	"os"

	"github.com/pkg/errors"
	"gopkg.in/yaml.v3"
)

// QueueSpecField represents a single field in a queue definition
type QueueSpecField struct {
	Name string `yaml:"name"`
	Type string `yaml:"type"`
}

// QueueSpecDef represents a single queue definition
type QueueSpecDef struct {
	ID     int              `yaml:"id"`
	Name   string           `yaml:"name"`
	Fields []QueueSpecField `yaml:"fields"`
}

// QueueSpec represents the full queue contract
type QueueSpec struct {
	Queues []QueueSpecDef `yaml:"queues"`
}

// validQueueFieldTypes contains allowed field types
var validQueueFieldTypes = map[string]bool{
	"int":     true,
	"int64":   true,
	"string":  true,
	"bool":    true,
	"[]byte":  true,
	"[]int":   true,
	"[]int64": true,
}

// ParseQueueSpec reads and parses a queue contract YAML file
func ParseQueueSpec(path string) (*QueueSpec, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, errors.Wrapf(err, "failed to read queue spec file: %s", path)
	}

	var spec QueueSpec
	if err := yaml.Unmarshal(data, &spec); err != nil {
		return nil, errors.Wrapf(err, "failed to parse queue spec file: %s", path)
	}

	if err := spec.Validate(); err != nil {
		return nil, errors.Wrapf(err, "invalid queue spec: %s", path)
	}

	return &spec, nil
}

// Validate checks the queue spec for errors
func (s *QueueSpec) Validate() error {
	if len(s.Queues) == 0 {
		return errors.New("no queues defined")
	}

	seenIDs := make(map[int]struct{})
	seenNames := make(map[string]struct{})

	for _, q := range s.Queues {
		if q.Name == "" {
			return errors.New("queue name is empty")
		}

		if _, exists := seenIDs[q.ID]; exists {
			return fmt.Errorf("duplicate queue id: %d", q.ID)
		}
		seenIDs[q.ID] = struct{}{}

		if _, exists := seenNames[q.Name]; exists {
			return fmt.Errorf("duplicate queue name: %s", q.Name)
		}
		seenNames[q.Name] = struct{}{}

		if len(q.Fields) == 0 {
			return fmt.Errorf("queue '%s' has no fields", q.Name)
		}

		seenFields := make(map[string]struct{})
		for _, f := range q.Fields {
			if f.Name == "" {
				return fmt.Errorf("queue '%s': field name is empty", q.Name)
			}

			if _, exists := seenFields[f.Name]; exists {
				return fmt.Errorf("queue '%s': duplicate field name: %s", q.Name, f.Name)
			}
			seenFields[f.Name] = struct{}{}

			if !validQueueFieldTypes[f.Type] {
				return fmt.Errorf("queue '%s': field '%s': invalid type '%s'", q.Name, f.Name, f.Type)
			}
		}
	}

	return nil
}
