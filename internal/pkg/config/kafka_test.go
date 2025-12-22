package config

import (
	"testing"
)

func TestKafkaTopic_IsValid(t *testing.T) {
	jsonSchemaMap := map[string]JSONSchema{
		"models": {
			Name: "models",
			Schemas: []JSONSchemaItem{
				{ID: "user", Path: "./user.schema.json", Type: "UserSchemaJson"},
				{ID: "audit", Path: "./audit.schema.json", Type: "AuditLogSchemaJson"},
			},
		},
		"events": {
			Name: "events",
			Schemas: []JSONSchemaItem{
				{ID: "event", Path: "./event.schema.json", Type: "EventSchemaJson"},
			},
		},
	}

	tests := []struct {
		name     string
		topic    KafkaTopic
		wantOK   bool
		wantMsg  string
		schemaFn func() map[string]JSONSchema
	}{
		{
			name: "valid topic with schema reference",
			topic: KafkaTopic{
				ID:     "user_events",
				Name:   "user.events",
				Schema: "models.user",
			},
			wantOK: true,
			schemaFn: func() map[string]JSONSchema {
				return jsonSchemaMap
			},
		},
		{
			name: "valid topic without schema (raw bytes)",
			topic: KafkaTopic{
				ID:   "raw_events",
				Name: "raw.events",
			},
			wantOK: true,
			schemaFn: func() map[string]JSONSchema {
				return jsonSchemaMap
			},
		},
		{
			name: "empty topic id",
			topic: KafkaTopic{
				ID:   "",
				Name: "events",
			},
			wantOK:  false,
			wantMsg: "Empty topic id",
			schemaFn: func() map[string]JSONSchema {
				return nil
			},
		},
		{
			name: "empty topic name",
			topic: KafkaTopic{
				ID:   "events",
				Name: "",
			},
			wantOK:  false,
			wantMsg: "Empty topic name",
			schemaFn: func() map[string]JSONSchema {
				return nil
			},
		},
		{
			name: "invalid schema format (no dot)",
			topic: KafkaTopic{
				ID:     "events",
				Name:   "events.topic",
				Schema: "models",
			},
			wantOK:  false,
			wantMsg: "Invalid schema format: expected 'schemaset.schemaid', got: models",
			schemaFn: func() map[string]JSONSchema {
				return jsonSchemaMap
			},
		},
		{
			name: "unknown schema set",
			topic: KafkaTopic{
				ID:     "events",
				Name:   "events.topic",
				Schema: "unknown.user",
			},
			wantOK:  false,
			wantMsg: "Unknown jsonschema reference: unknown",
			schemaFn: func() map[string]JSONSchema {
				return jsonSchemaMap
			},
		},
		{
			name: "unknown schema id in existing set",
			topic: KafkaTopic{
				ID:     "events",
				Name:   "events.topic",
				Schema: "models.unknown_id",
			},
			wantOK:  false,
			wantMsg: "Unknown schema id 'unknown_id' in schema set 'models'",
			schemaFn: func() map[string]JSONSchema {
				return jsonSchemaMap
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotOK, gotMsg := tt.topic.IsValid(tt.schemaFn())

			if gotOK != tt.wantOK {
				t.Errorf("KafkaTopic.IsValid() ok = %v, want %v", gotOK, tt.wantOK)
			}

			if !tt.wantOK && gotMsg != tt.wantMsg {
				t.Errorf("KafkaTopic.IsValid() msg = %q, want %q", gotMsg, tt.wantMsg)
			}
		})
	}
}

func TestKafka_IsValid(t *testing.T) {
	jsonSchemaMap := map[string]JSONSchema{
		"models": {
			Name: "models",
			Schemas: []JSONSchemaItem{
				{ID: "user", Path: "./user.schema.json", Type: "UserSchemaJson"},
			},
		},
	}

	validTopicWithSchema := KafkaTopic{
		ID:     "user_events",
		Name:   "user.events",
		Schema: "models.user",
	}

	validTopicRaw := KafkaTopic{
		ID:   "raw_events",
		Name: "raw.events",
	}

	tests := []struct {
		name    string
		kafka   Kafka
		wantOK  bool
		wantMsg string
	}{
		{
			name: "valid producer with schema topic",
			kafka: Kafka{
				Name:   "events_producer",
				Type:   KafkaTypeProducer,
				Client: "events",
				Topics: []KafkaTopic{validTopicWithSchema},
			},
			wantOK: true,
		},
		{
			name: "valid producer with raw topic",
			kafka: Kafka{
				Name:   "raw_producer",
				Type:   KafkaTypeProducer,
				Client: "raw",
				Topics: []KafkaTopic{validTopicRaw},
			},
			wantOK: true,
		},
		{
			name: "valid consumer",
			kafka: Kafka{
				Name:   "events_consumer",
				Type:   KafkaTypeConsumer,
				Client: "events",
				Group:  "my_group",
				Topics: []KafkaTopic{validTopicWithSchema},
			},
			wantOK: true,
		},
		{
			name: "empty name",
			kafka: Kafka{
				Name:   "",
				Type:   KafkaTypeProducer,
				Topics: []KafkaTopic{validTopicWithSchema},
			},
			wantOK:  false,
			wantMsg: "Empty name",
		},
		{
			name: "invalid type",
			kafka: Kafka{
				Name:   "test",
				Type:   "invalid",
				Topics: []KafkaTopic{validTopicWithSchema},
			},
			wantOK:  false,
			wantMsg: "Invalid type: must be 'producer' or 'consumer'",
		},
		{
			name: "empty client",
			kafka: Kafka{
				Name:   "test",
				Type:   KafkaTypeProducer,
				Topics: []KafkaTopic{validTopicWithSchema},
			},
			wantOK:  false,
			wantMsg: "Empty client",
		},
		{
			name: "empty topics",
			kafka: Kafka{
				Name:   "test",
				Type:   KafkaTypeProducer,
				Client: "test",
				Topics: []KafkaTopic{},
			},
			wantOK:  false,
			wantMsg: "Empty topics",
		},
		{
			name: "consumer without group",
			kafka: Kafka{
				Name:   "test",
				Type:   KafkaTypeConsumer,
				Client: "test",
				Topics: []KafkaTopic{validTopicWithSchema},
			},
			wantOK:  false,
			wantMsg: "Consumer requires group",
		},
		{
			name: "invalid topic",
			kafka: Kafka{
				Name:   "test",
				Type:   KafkaTypeProducer,
				Client: "test",
				Topics: []KafkaTopic{{ID: "", Name: "test"}},
			},
			wantOK:  false,
			wantMsg: "topic test: Empty topic id",
		},
		{
			name: "custom driver without all fields",
			kafka: Kafka{
				Name:   "test",
				Type:   KafkaTypeProducer,
				Driver: KafkaDriverCustom,
				Client: "test",
				Topics: []KafkaTopic{validTopicWithSchema},
			},
			wantOK:  false,
			wantMsg: "Custom driver requires driver_import, driver_package, driver_obj",
		},
		{
			name: "valid custom driver",
			kafka: Kafka{
				Name:          "test",
				Type:          KafkaTypeProducer,
				Driver:        KafkaDriverCustom,
				DriverImport:  "github.com/example/driver",
				DriverPackage: "driver",
				DriverObj:     "Producer",
				Client:        "test",
				Topics:        []KafkaTopic{validTopicWithSchema},
			},
			wantOK: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotOK, gotMsg := tt.kafka.IsValid(jsonSchemaMap)

			if gotOK != tt.wantOK {
				t.Errorf("Kafka.IsValid() ok = %v, want %v", gotOK, tt.wantOK)
			}

			if !tt.wantOK && gotMsg != tt.wantMsg {
				t.Errorf("Kafka.IsValid() msg = %q, want %q", gotMsg, tt.wantMsg)
			}
		})
	}
}

