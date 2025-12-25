package dockerintegration

import (
	"context"
	"fmt"
	"time"

	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/wait"
)

const telegramMockServiceName = "telegram-mock"

// TelegramMockContainer is a container running a mock Telegram Bot API server
type TelegramMockContainer struct {
	testcontainers.Container
	URL string
}

// RunTelegramMock creates and starts a mock Telegram Bot API server container
func RunTelegramMock(ctx context.Context, opts ...testcontainers.ContainerCustomizer) (*TelegramMockContainer, error) {
	// Create a simple HTTP server that responds to Telegram Bot API requests
	// Using a lightweight alpine image with a simple HTTP server
	req := testcontainers.ContainerRequest{
		Image:        "python:3.11-alpine",
		ExposedPorts: []string{"8080/tcp"},
		Cmd: []string{
			"python", "-c", telegramMockServerCode,
		},
		WaitingFor: wait.ForHTTP("/").WithPort("8080").WithStartupTimeout(30 * time.Second),
	}

	genericReq := testcontainers.GenericContainerRequest{
		ContainerRequest: req,
		Started:          true,
	}

	for _, opt := range opts {
		if err := opt.Customize(&genericReq); err != nil {
			return nil, fmt.Errorf("failed to apply customizer: %w", err)
		}
	}

	container, err := testcontainers.GenericContainer(ctx, genericReq)
	if err != nil {
		return nil, fmt.Errorf("failed to create telegram mock container: %w", err)
	}

	host, err := container.Host(ctx)
	if err != nil {
		container.Terminate(ctx)
		return nil, fmt.Errorf("failed to get host: %w", err)
	}

	port, err := container.MappedPort(ctx, "8080")
	if err != nil {
		container.Terminate(ctx)
		return nil, fmt.Errorf("failed to get port: %w", err)
	}

	url := fmt.Sprintf("http://%s:%s", host, port.Port())

	return &TelegramMockContainer{
		Container: container,
		URL:       url,
	}, nil
}

// GetInternalURL returns the URL accessible from within the Docker network
func (t *TelegramMockContainer) GetInternalURL(ctx context.Context) (string, error) {
	containerIP, err := t.ContainerIP(ctx)
	if err != nil {
		return "", err
	}
	return fmt.Sprintf("http://%s:8080", containerIP), nil
}

// telegramMockServerCode is a simple Python HTTP server that mocks Telegram Bot API
const telegramMockServerCode = `
import http.server
import json
import re

class TelegramMockHandler(http.server.BaseHTTPRequestHandler):
    def do_GET(self):
        self.send_response(200)
        self.send_header('Content-type', 'text/plain')
        self.end_headers()
        self.wfile.write(b'Telegram Mock Server')

    def do_POST(self):
        # Match /bot<token>/getMe
        if re.match(r'/bot[^/]+/getMe', self.path):
            response = {
                "ok": True,
                "result": {
                    "id": 123456789,
                    "is_bot": True,
                    "first_name": "TestBot",
                    "username": "test_bot",
                    "can_join_groups": True,
                    "can_read_all_group_messages": False,
                    "supports_inline_queries": False
                }
            }
            self.send_response(200)
            self.send_header('Content-type', 'application/json')
            self.end_headers()
            self.wfile.write(json.dumps(response).encode())
        # Match /bot<token>/getUpdates
        elif re.match(r'/bot[^/]+/getUpdates', self.path):
            response = {
                "ok": True,
                "result": []
            }
            self.send_response(200)
            self.send_header('Content-type', 'application/json')
            self.end_headers()
            self.wfile.write(json.dumps(response).encode())
        else:
            # Default response for unknown endpoints
            response = {
                "ok": True,
                "result": {}
            }
            self.send_response(200)
            self.send_header('Content-type', 'application/json')
            self.end_headers()
            self.wfile.write(json.dumps(response).encode())

    def log_message(self, format, *args):
        pass  # Suppress logging

server = http.server.HTTPServer(('0.0.0.0', 8080), TelegramMockHandler)
print('Telegram Mock Server started on port 8080')
server.serve_forever()
`
