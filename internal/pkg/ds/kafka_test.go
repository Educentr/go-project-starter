package ds

import (
	"testing"
)

func TestKafkaConfig_IsCustomDriver(t *testing.T) {
	tests := []struct {
		name   string
		driver string
		want   bool
	}{
		{
			name:   "custom driver",
			driver: KafkaDriverCustom,
			want:   true,
		},
		{
			name:   "segmentio driver",
			driver: "segmentio",
			want:   false,
		},
		{
			name:   "empty driver",
			driver: "",
			want:   false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			k := KafkaConfig{Driver: tt.driver}

			if got := k.IsCustomDriver(); got != tt.want {
				t.Errorf("IsCustomDriver() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestKafkaConfig_GetImport(t *testing.T) {
	modulePath := "github.com/myorg/myservice"

	tests := []struct {
		name       string
		kafka      KafkaConfig
		modulePath string
		want       string
	}{
		{
			name: "segmentio driver",
			kafka: KafkaConfig{
				Name:   "events_producer",
				Driver: "segmentio",
			},
			modulePath: modulePath,
			want:       modulePath + "/pkg/drivers/kafka/events_producer",
		},
		{
			name: "custom driver",
			kafka: KafkaConfig{
				Name:         "custom_producer",
				Driver:       KafkaDriverCustom,
				DriverImport: "github.com/myorg/kafka-driver",
			},
			modulePath: modulePath,
			want:       "github.com/myorg/kafka-driver",
		},
		{
			name: "empty driver defaults to generated path",
			kafka: KafkaConfig{
				Name:   "my_producer",
				Driver: "",
			},
			modulePath: modulePath,
			want:       modulePath + "/pkg/drivers/kafka/my_producer",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.kafka.GetImport(tt.modulePath); got != tt.want {
				t.Errorf("GetImport() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestKafkaConfig_GetPackage(t *testing.T) {
	tests := []struct {
		name  string
		kafka KafkaConfig
		want  string
	}{
		{
			name: "segmentio driver",
			kafka: KafkaConfig{
				Name:   "Events_Producer",
				Driver: "segmentio",
			},
			want: "events_producer",
		},
		{
			name: "custom driver",
			kafka: KafkaConfig{
				Name:          "custom_producer",
				Driver:        KafkaDriverCustom,
				DriverPackage: "kafkadriver",
			},
			want: "kafkadriver",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.kafka.GetPackage(); got != tt.want {
				t.Errorf("GetPackage() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestKafkaConfig_GetObjName(t *testing.T) {
	tests := []struct {
		name  string
		kafka KafkaConfig
		want  string
	}{
		{
			name: "segmentio driver",
			kafka: KafkaConfig{
				Name:   "events_producer",
				Driver: "segmentio",
			},
			want: KafkaObjNameProducer,
		},
		{
			name: "custom driver",
			kafka: KafkaConfig{
				Name:      "custom_producer",
				Driver:    KafkaDriverCustom,
				DriverObj: "MyProducer",
			},
			want: "MyProducer",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.kafka.GetObjName(); got != tt.want {
				t.Errorf("GetObjName() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestApp_HasKafkaProducers(t *testing.T) {
	tests := []struct {
		name string
		app  App
		want bool
	}{
		{
			name: "has producer",
			app: App{
				Kafka: KafkaConfigs{
					"producer1": {Type: KafkaTypeProducer},
				},
			},
			want: true,
		},
		{
			name: "only consumer",
			app: App{
				Kafka: KafkaConfigs{
					"consumer1": {Type: KafkaTypeConsumer},
				},
			},
			want: false,
		},
		{
			name: "mixed",
			app: App{
				Kafka: KafkaConfigs{
					"producer1": {Type: KafkaTypeProducer},
					"consumer1": {Type: KafkaTypeConsumer},
				},
			},
			want: true,
		},
		{
			name: "empty",
			app:  App{},
			want: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.app.HasKafkaProducers(); got != tt.want {
				t.Errorf("HasKafkaProducers() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestApp_GetKafkaProducers(t *testing.T) {
	app := App{
		Kafka: KafkaConfigs{
			"producer1": {Name: "producer1", Type: KafkaTypeProducer},
			"consumer1": {Name: "consumer1", Type: KafkaTypeConsumer},
			"producer2": {Name: "producer2", Type: KafkaTypeProducer},
		},
	}

	producers := app.GetKafkaProducers()

	if len(producers) != 2 {
		t.Errorf("GetKafkaProducers() returned %d items, want 2", len(producers))
	}

	// Verify only producers
	for _, p := range producers {
		if p.Type != KafkaTypeProducer {
			t.Errorf("GetKafkaProducers() returned non-producer: %v", p.Type)
		}
	}

	// Verify sorted by name
	if len(producers) == 2 && producers[0].Name > producers[1].Name {
		t.Errorf("GetKafkaProducers() not sorted: %v, %v", producers[0].Name, producers[1].Name)
	}
}

func TestApp_KafkaImports(t *testing.T) {
	modulePath := "github.com/myorg/myservice"

	app := App{
		Kafka: KafkaConfigs{
			"producer1": {
				Name:   "producer1",
				Type:   KafkaTypeProducer,
				Driver: "segmentio",
			},
			"consumer1": {
				Name:   "consumer1",
				Type:   KafkaTypeConsumer,
				Driver: "segmentio",
			},
		},
	}

	imports := app.KafkaImports(modulePath)

	// Should only include producer imports
	if len(imports) != 1 {
		t.Errorf("KafkaImports() returned %d items, want 1", len(imports))
	}

	expected := modulePath + "/pkg/drivers/kafka/producer1"
	if len(imports) > 0 && imports[0] != expected {
		t.Errorf("KafkaImports() = %v, want %v", imports[0], expected)
	}
}

func TestKafkaConstants(t *testing.T) {
	// Verify constants are set correctly
	if KafkaTypeProducer != "producer" {
		t.Errorf("KafkaTypeProducer = %q, want %q", KafkaTypeProducer, "producer")
	}

	if KafkaTypeConsumer != "consumer" {
		t.Errorf("KafkaTypeConsumer = %q, want %q", KafkaTypeConsumer, "consumer")
	}

	if KafkaDriverCustom != "custom" {
		t.Errorf("KafkaDriverCustom = %q, want %q", KafkaDriverCustom, "custom")
	}

	if KafkaObjNameProducer != "Producer" {
		t.Errorf("KafkaObjNameProducer = %q, want %q", KafkaObjNameProducer, "Producer")
	}
}
