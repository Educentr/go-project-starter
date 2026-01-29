package config

import (
	"testing"
)

func TestKafkaEvent_IsValid(t *testing.T) {
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
		event    KafkaEvent
		wantOK   bool
		wantMsg  string
		schemaFn func() map[string]JSONSchema
	}{
		{
			name: "valid event with schema reference",
			event: KafkaEvent{
				Name:   "user_events",
				Schema: "models.user",
			},
			wantOK: true,
			schemaFn: func() map[string]JSONSchema {
				return jsonSchemaMap
			},
		},
		{
			name: "valid event without schema (raw bytes)",
			event: KafkaEvent{
				Name: "raw_events",
			},
			wantOK: true,
			schemaFn: func() map[string]JSONSchema {
				return jsonSchemaMap
			},
		},
		{
			name: "empty event name",
			event: KafkaEvent{
				Name: "",
			},
			wantOK:  false,
			wantMsg: "Empty event name",
			schemaFn: func() map[string]JSONSchema {
				return nil
			},
		},
		{
			name: "invalid schema format (no dot)",
			event: KafkaEvent{
				Name:   "events",
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
			event: KafkaEvent{
				Name:   "events",
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
			event: KafkaEvent{
				Name:   "events",
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
			gotOK, gotMsg := tt.event.IsValid(tt.schemaFn())

			if gotOK != tt.wantOK {
				t.Errorf("KafkaEvent.IsValid() ok = %v, want %v", gotOK, tt.wantOK)
			}

			if !tt.wantOK && gotMsg != tt.wantMsg {
				t.Errorf("KafkaEvent.IsValid() msg = %q, want %q", gotMsg, tt.wantMsg)
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

	validEventWithSchema := KafkaEvent{
		Name:   "user_events",
		Schema: "models.user",
	}

	validEventRaw := KafkaEvent{
		Name: "raw_events",
	}

	tests := []struct {
		name    string
		kafka   Kafka
		wantOK  bool
		wantMsg string
	}{
		{
			name: "valid producer with schema event",
			kafka: Kafka{
				Name:   "events_producer",
				Type:   KafkaTypeProducer,
				Client: "events",
				Events: []KafkaEvent{validEventWithSchema},
			},
			wantOK: true,
		},
		{
			name: "valid producer with raw event",
			kafka: Kafka{
				Name:   "raw_producer",
				Type:   KafkaTypeProducer,
				Client: "raw",
				Events: []KafkaEvent{validEventRaw},
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
				Events: []KafkaEvent{validEventWithSchema},
			},
			wantOK: true,
		},
		{
			name: "empty name",
			kafka: Kafka{
				Name:   "",
				Type:   KafkaTypeProducer,
				Events: []KafkaEvent{validEventWithSchema},
			},
			wantOK:  false,
			wantMsg: "Empty name",
		},
		{
			name: "invalid type",
			kafka: Kafka{
				Name:   "test",
				Type:   "invalid",
				Events: []KafkaEvent{validEventWithSchema},
			},
			wantOK:  false,
			wantMsg: "Invalid type: must be 'producer' or 'consumer'",
		},
		{
			name: "empty client",
			kafka: Kafka{
				Name:   "test",
				Type:   KafkaTypeProducer,
				Events: []KafkaEvent{validEventWithSchema},
			},
			wantOK:  false,
			wantMsg: "Empty client",
		},
		{
			name: "empty events",
			kafka: Kafka{
				Name:   "test",
				Type:   KafkaTypeProducer,
				Client: "test",
				Events: []KafkaEvent{},
			},
			wantOK:  false,
			wantMsg: "Empty events",
		},
		{
			name: "consumer without group",
			kafka: Kafka{
				Name:   "test",
				Type:   KafkaTypeConsumer,
				Client: "test",
				Events: []KafkaEvent{validEventWithSchema},
			},
			wantOK:  false,
			wantMsg: "Consumer requires group",
		},
		{
			name: "invalid event",
			kafka: Kafka{
				Name:   "test",
				Type:   KafkaTypeProducer,
				Client: "test",
				Events: []KafkaEvent{{Name: ""}},
			},
			wantOK:  false,
			wantMsg: "event : Empty event name",
		},
		{
			name: "custom driver without all fields",
			kafka: Kafka{
				Name:   "test",
				Type:   KafkaTypeProducer,
				Driver: KafkaDriverCustom,
				Client: "test",
				Events: []KafkaEvent{validEventWithSchema},
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
				Events:        []KafkaEvent{validEventWithSchema},
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
