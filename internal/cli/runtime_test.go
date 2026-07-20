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
record decimal_request
begin
    field amount decimal
end
enum app_error
begin
    case invalid_request
    case user_not_found
    case database_failure
end
function exact_total
output decimal
begin
    return call add 0.1 0.2
end
function repeating_decimal
output decimal
begin
    return call divide 1 3
end
function fractional_remainder
output float32
begin
    return call remainder 5.5 2
end
route health
method get
path /health
begin
    respond text 200 ready
end
route money
method get
path /money
begin
    let total to be call exact_total
    respond json 200 total
end
route decimal_echo
method post
path /decimal
begin
    let payload to be try call json_decode decimal_request request_body
    let amount to be get payload amount
    respond json 200 amount
end
route repeating
method get
path /repeating
begin
    let value to be call repeating_decimal
    respond json 200 value
end
route remainder
method get
path /remainder
begin
    let value to be call fractional_remainder
    respond json 200 value
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
route typed_success
method get
path /typed-success
output result string app_error
begin
    let value to be text ready
    return call ok value
end
route typed_parse
method get
path /typed-parse/{id}
output result uuid app_error
begin
    let parsed_id to be try call parse_uuid id
    return call ok parsed_id
end
route typed_missing
method get
path /typed-missing
output result string app_error
begin
    return call error user_not_found
end
route typed_database_failure
method get
path /typed-database-failure
output result string app_error
begin
    return call error database_failure
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
	stopServer := func() {
		if server.Process != nil {
			_ = server.Process.Kill()
			_, _ = server.Process.Wait()
			server.Process = nil
		}
	}
	t.Cleanup(stopServer)

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

	moneyResponse, err := client.Get(baseURL + "/money")
	if err != nil {
		t.Fatal(err)
	}
	moneyBody, err := io.ReadAll(moneyResponse.Body)
	_ = moneyResponse.Body.Close()
	if err != nil {
		t.Fatal(err)
	}
	if moneyResponse.StatusCode != http.StatusOK || strings.TrimSpace(string(moneyBody)) != "0.3" {
		t.Fatalf("unexpected decimal response: status=%d body=%q", moneyResponse.StatusCode, moneyBody)
	}

	decimalResponse, err := client.Post(baseURL+"/decimal", "application/json", strings.NewReader(`{"amount":1.2300}`))
	if err != nil {
		t.Fatal(err)
	}
	decimalBody, err := io.ReadAll(decimalResponse.Body)
	_ = decimalResponse.Body.Close()
	if err != nil {
		t.Fatal(err)
	}
	if decimalResponse.StatusCode != http.StatusOK || strings.TrimSpace(string(decimalBody)) != "1.23" {
		t.Fatalf("unexpected decoded decimal response: status=%d body=%q", decimalResponse.StatusCode, decimalBody)
	}

	repeatingResponse, err := client.Get(baseURL + "/repeating")
	if err != nil {
		t.Fatal(err)
	}
	_, _ = io.Copy(io.Discard, repeatingResponse.Body)
	_ = repeatingResponse.Body.Close()
	if repeatingResponse.StatusCode != http.StatusInternalServerError {
		t.Fatalf("non-terminating decimal returned status %d, want 500", repeatingResponse.StatusCode)
	}

	remainderResponse, err := client.Get(baseURL + "/remainder")
	if err != nil {
		t.Fatal(err)
	}
	remainderBody, err := io.ReadAll(remainderResponse.Body)
	_ = remainderResponse.Body.Close()
	if err != nil {
		t.Fatal(err)
	}
	if remainderResponse.StatusCode != http.StatusOK || strings.TrimSpace(string(remainderBody)) != "1.5" {
		t.Fatalf("unexpected float remainder response: status=%d body=%q", remainderResponse.StatusCode, remainderBody)
	}

	invalidResponse, err := client.Post(baseURL+"/validate", "application/json", strings.NewReader(`{"id":`))
	if err != nil {
		t.Fatal(err)
	}
	_, _ = io.Copy(io.Discard, invalidResponse.Body)
	_ = invalidResponse.Body.Close()
	if invalidResponse.StatusCode != http.StatusBadRequest {
		t.Fatalf("invalid JSON returned status %d, want 400", invalidResponse.StatusCode)
	}

	assertHTTPResponse(t, client, http.MethodGet, baseURL+"/typed-success", "", http.StatusOK, `"ready"`)
	assertHTTPResponse(t, client, http.MethodGet, baseURL+"/typed-parse/550e8400-e29b-41d4-a716-446655440000", "", http.StatusOK, `"550e8400-e29b-41d4-a716-446655440000"`)
	assertHTTPResponse(t, client, http.MethodGet, baseURL+"/typed-parse/not-a-uuid", "", http.StatusBadRequest, "invalid_request")
	assertHTTPResponse(t, client, http.MethodGet, baseURL+"/typed-missing", "", http.StatusNotFound, "user_not_found")
	assertHTTPResponse(t, client, http.MethodGet, baseURL+"/typed-database-failure", "", http.StatusInternalServerError, "database_failure")
	stopServer()
}

func TestBuildPostgresService(t *testing.T) {
	directory := t.TempDir()
	if err := os.WriteFile(filepath.Join(directory, "verba.toml"), []byte(`name = "postgres_test"
version = "0.1.0"
target = "go"

[database]
dialect = "postgres"
schema = "schema.sql"
`), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(directory, "schema.sql"), []byte("CREATE TABLE users (id text PRIMARY KEY, name text NOT NULL, retention interval NOT NULL);\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(directory, "main.vrb"), []byte(`module postgres_test
use http
use json
use sql postgres
use time
record user
begin
    field id string
    field name string
    field retention duration
end
embed sql find_user until end_sql
SELECT id, name, retention FROM users WHERE id = :id;
end_sql
route find
method get
path /users/{id}
begin
    let found to be try call sql_optional find_user
    begin
        with id id
    end
    respond json 200 found
end
`), 0o644); err != nil {
		t.Fatal(err)
	}
	executable := filepath.Join(directory, "postgres-service")
	if runtime.GOOS == "windows" {
		executable += ".exe"
	}
	var output bytes.Buffer
	command := &CLI{Stdout: &output, Stderr: &output, Stdin: strings.NewReader("")}
	if code := command.Run([]string{"build", "-o", executable, directory}); code != 0 {
		t.Fatalf("build exited with %d:\n%s", code, output.String())
	}
	if info, err := os.Stat(executable); err != nil || info.IsDir() {
		t.Fatalf("generated executable is unavailable: %v", err)
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

func assertHTTPResponse(t *testing.T, client *http.Client, method, url, body string, status int, expectedBody string) {
	t.Helper()
	request, err := http.NewRequest(method, url, strings.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	response, err := client.Do(request)
	if err != nil {
		t.Fatal(err)
	}
	responseBody, err := io.ReadAll(response.Body)
	_ = response.Body.Close()
	if err != nil {
		t.Fatal(err)
	}
	if response.StatusCode != status || strings.TrimSpace(string(responseBody)) != expectedBody {
		t.Fatalf("unexpected response for %s: status=%d body=%q", url, response.StatusCode, responseBody)
	}
}
