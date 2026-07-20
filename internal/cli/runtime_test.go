package cli

import (
	"bytes"
	"io"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"
)

func TestBuildAndRunHTTPService(t *testing.T) {
	directory := t.TempDir()
	sourcePath := filepath.Join(directory, "main.vrb")
	source := []byte(`module runtime_test
use http
use json
use uuid
record uuid_request
begin
    field id string
end
route health
method get
path /health
begin
    respond text 200 ready
end
route validate
method post
path /validate
begin
    let payload to be try call json_decode uuid_request request_body
    let raw_id to be get payload id
    let parsed_id to be try call parse_uuid raw_id
    respond json 200 parsed_id
end
`)
	if err := os.WriteFile(sourcePath, source, 0o644); err != nil {
		t.Fatal(err)
	}
	executable := filepath.Join(directory, "service")
	if runtime.GOOS == "windows" {
		executable += ".exe"
	}
	var buildOutput bytes.Buffer
	command := &CLI{Stdout: &buildOutput, Stderr: &buildOutput, Stdin: strings.NewReader("")}
	if code := command.Run([]string{"build", "-o", executable, sourcePath}); code != 0 {
		t.Fatalf("build exited with %d:\n%s", code, buildOutput.String())
	}

	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	address := listener.Addr().String()
	if err := listener.Close(); err != nil {
		t.Fatal(err)
	}

	var serverOutput bytes.Buffer
	server := exec.Command(executable)
	server.Env = append(os.Environ(), "VERBA_ADDRESS="+address)
	server.Stdout = &serverOutput
	server.Stderr = &serverOutput
	if err := server.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if server.Process != nil {
			_ = server.Process.Kill()
			_, _ = server.Process.Wait()
		}
	})

	client := &http.Client{Timeout: 2 * time.Second}
	baseURL := "http://" + address
	waitForHTTP(t, client, baseURL+"/health", &serverOutput)

	validResponse, err := client.Post(baseURL+"/validate", "application/json", strings.NewReader(`{"id":"550e8400-e29b-41d4-a716-446655440000"}`))
	if err != nil {
		t.Fatal(err)
	}
	validBody, err := io.ReadAll(validResponse.Body)
	_ = validResponse.Body.Close()
	if err != nil {
		t.Fatal(err)
	}
	if validResponse.StatusCode != http.StatusOK || strings.TrimSpace(string(validBody)) != `"550e8400-e29b-41d4-a716-446655440000"` {
		t.Fatalf("unexpected valid response: status=%d body=%q", validResponse.StatusCode, validBody)
	}

	invalidResponse, err := client.Post(baseURL+"/validate", "application/json", strings.NewReader(`{"id":`))
	if err != nil {
		t.Fatal(err)
	}
	_, _ = io.Copy(io.Discard, invalidResponse.Body)
	_ = invalidResponse.Body.Close()
	if invalidResponse.StatusCode != http.StatusInternalServerError {
		t.Fatalf("invalid JSON returned status %d, want 500", invalidResponse.StatusCode)
	}
}

func waitForHTTP(t *testing.T, client *http.Client, url string, serverOutput *bytes.Buffer) {
	t.Helper()
	deadline := time.Now().Add(10 * time.Second)
	for time.Now().Before(deadline) {
		response, err := client.Get(url)
		if err == nil {
			_, _ = io.Copy(io.Discard, response.Body)
			_ = response.Body.Close()
			if response.StatusCode == http.StatusOK {
				return
			}
		}
		time.Sleep(100 * time.Millisecond)
	}
	t.Fatalf("generated server did not become ready:\n%s", serverOutput.String())
}
