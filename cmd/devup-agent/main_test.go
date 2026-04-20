package main

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"devup/internal/api"
)

func TestHandleRunRejectsOversizedJSONBody(t *testing.T) {
	handler := handleRun(make(chan struct{}, 1), make(map[string]*runResult), &sync.RWMutex{})
	req := httptest.NewRequest(http.MethodPost, "/run", strings.NewReader(oversizedRunRequestBody()))
	rec := httptest.NewRecorder()

	handler(rec, req)

	if rec.Code != http.StatusRequestEntityTooLarge {
		t.Fatalf("expected status %d, got %d: %s", http.StatusRequestEntityTooLarge, rec.Code, rec.Body.String())
	}
}

func TestHandleStartRejectsOversizedJSONBody(t *testing.T) {
	req := httptest.NewRequest(http.MethodPost, "/start", strings.NewReader(oversizedStartRequestBody()))
	rec := httptest.NewRecorder()

	handleStart(rec, req)

	if rec.Code != http.StatusRequestEntityTooLarge {
		t.Fatalf("expected status %d, got %d: %s", http.StatusRequestEntityTooLarge, rec.Code, rec.Body.String())
	}
}

func TestBuildEnvNormalizesMacOSPathsAndPreservesLinuxOverrides(t *testing.T) {
	t.Setenv("HOME", "/Users/host")
	t.Setenv("XDG_CACHE_HOME", "/Users/host/.cache")

	env := envSliceToMap(buildEnv(map[string]string{
		"HOME":             "/Users/request",
		"NPM_CONFIG_CACHE": "/tmp/custom-npm-cache",
		"CUSTOM":           "value",
	}))

	if env["HOME"] != defaultHome {
		t.Fatalf("expected HOME to be normalized to %q, got %q", defaultHome, env["HOME"])
	}
	if env["XDG_CACHE_HOME"] != defaultHome+"/.cache" {
		t.Fatalf("expected XDG_CACHE_HOME to be normalized, got %q", env["XDG_CACHE_HOME"])
	}
	if env["NPM_CONFIG_CACHE"] != "/tmp/custom-npm-cache" {
		t.Fatalf("expected Linux override to win, got %q", env["NPM_CONFIG_CACHE"])
	}
	if env["CUSTOM"] != "value" {
		t.Fatalf("expected custom env var to be preserved, got %q", env["CUSTOM"])
	}
}

func TestMergeMiseEnvPrependsPathAndOverridesValues(t *testing.T) {
	merged := envSliceToMap(mergeMiseEnv([]string{
		"PATH=/usr/bin:/bin",
		"FOO=old",
	}, map[string]string{
		"PATH": "/mise/bin",
		"FOO":  "new",
		"BAR":  "added",
	}))

	if merged["PATH"] != "/mise/bin:/usr/bin:/bin" {
		t.Fatalf("expected PATH to be prepended, got %q", merged["PATH"])
	}
	if merged["FOO"] != "new" {
		t.Fatalf("expected FOO to be overridden, got %q", merged["FOO"])
	}
	if merged["BAR"] != "added" {
		t.Fatalf("expected BAR to be added, got %q", merged["BAR"])
	}
}

func TestStreamResultAppendsExitCodeLine(t *testing.T) {
	rec := httptest.NewRecorder()

	streamResult(rec, 17, "hello")

	if body := rec.Body.String(); body != "hello\n"+exitCodePrefix+"17\n" {
		t.Fatalf("unexpected stream body %q", body)
	}
}

func TestWaitForDoneTimesOutWhenChannelStaysOpen(t *testing.T) {
	done := make(chan struct{})
	if waitForDone(done, 0) {
		t.Fatal("expected open channel to time out")
	}
	close(done)
	if !waitForDone(done, 0) {
		t.Fatal("expected closed channel to return immediately")
	}
}

func oversizedRunRequestBody() string {
	req := api.RunRequest{
		RequestID: "req-1",
		Cmd:       []string{"echo", "ok"},
		Env: map[string]string{
			"BIG": strings.Repeat("x", jsonRequestMaxBytes),
		},
	}
	return marshalRequest(req)
}

func oversizedStartRequestBody() string {
	req := api.StartRequest{
		RequestID: "req-2",
		Cmd:       []string{"echo", "ok"},
		Env: map[string]string{
			"BIG": strings.Repeat("x", jsonRequestMaxBytes),
		},
	}
	return marshalRequest(req)
}

func marshalRequest(v any) string {
	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	if err := enc.Encode(v); err != nil {
		panic(err)
	}
	return buf.String()
}

func envSliceToMap(env []string) map[string]string {
	out := make(map[string]string, len(env))
	for _, entry := range env {
		parts := strings.SplitN(entry, "=", 2)
		if len(parts) == 2 {
			out[parts[0]] = parts[1]
		}
	}
	return out
}
