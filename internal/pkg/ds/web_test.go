package ds

import "testing"

// TestWebApp_StageFrom covers the FROM instruction of the frontend build stage
// in both directions: pinned to the build host when the application declares
// target platforms, and exactly as before when it does not.
func TestWebApp_StageFrom(t *testing.T) {
	web := WebApp{Name: "admin", NodeImage: "node:22-alpine"}

	// An unpinned stage of a multi-platform build would be built once per
	// target platform, under emulation of each.
	if got, want := web.StageFrom(true), "FROM --platform=$BUILDPLATFORM node:22-alpine AS web-admin"; got != want {
		t.Errorf("StageFrom(true) = %q, want %q", got, want)
	}

	// No declared platforms: the build passes no --platform, so the pin would
	// change the Dockerfile of an existing project without changing how it is
	// built.
	if got, want := web.StageFrom(false), "FROM node:22-alpine AS web-admin"; got != want {
		t.Errorf("StageFrom(false) = %q, want %q", got, want)
	}
}
