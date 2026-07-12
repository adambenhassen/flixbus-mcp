//go:build smoke

// Package smoke exercises the built flixbus-mcp binary end to end against the
// live FlixBus API: the stdio transport over real JSON-RPC, and the HTTP
// transport with stdin closed (the daemon rule). Run with:
//
//	go test -tags smoke -timeout 5m ./smoke
package smoke_test

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
	"testing"
	"time"
)

var binary string

func TestMain(m *testing.M) {
	os.Exit(testMain(m)) // os.Exit skips defers, so the work lives in testMain
}

func testMain(m *testing.M) int {
	dir, err := os.MkdirTemp("", "flixbus-smoke")
	if err != nil {
		fmt.Fprintf(os.Stderr, "mkdtemp: %v\n", err)
		return 1
	}
	defer func() {
		if err := os.RemoveAll(dir); err != nil {
			fmt.Fprintf(os.Stderr, "cleanup: %v\n", err)
		}
	}()
	binary = filepath.Join(dir, "flixbus-mcp")
	build := exec.CommandContext(context.Background(), "go", "build", "-o", binary, "../cmd/flixbus-mcp") //nolint:gosec // builds our own package into a temp dir
	build.Stderr = os.Stderr
	if err := build.Run(); err != nil {
		fmt.Fprintf(os.Stderr, "build: %v\n", err)
		return 1
	}
	return m.Run()
}

// rpc frames one JSON-RPC message per line, the stdio transport's wire format.
func rpc(id int, method string, params string) string {
	msg := fmt.Sprintf(`{"jsonrpc":"2.0","id":%d,"method":%q,"params":%s}`, id, method, params)
	if id == 0 { // notification
		msg = fmt.Sprintf(`{"jsonrpc":"2.0","method":%q}`, method)
	}
	return msg + "\n"
}

func TestStdioLiveSearch(t *testing.T) {
	cmd := exec.CommandContext(t.Context(), binary, "-transport", "stdio")
	stdin, err := cmd.StdinPipe()
	if err != nil {
		t.Fatalf("stdin pipe: %v", err)
	}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		t.Fatalf("stdout pipe: %v", err)
	}
	cmd.Stderr = io.Discard
	if err := cmd.Start(); err != nil {
		t.Fatalf("start: %v", err)
	}
	defer func() {
		if cmd.ProcessState == nil {
			if err := cmd.Process.Kill(); err != nil {
				t.Logf("kill: %v", err)
			}
			if err := cmd.Wait(); err != nil {
				t.Logf("wait after kill: %v", err)
			}
		}
	}()

	date := time.Now().AddDate(0, 0, 14).Format("2006-01-02")
	writes := rpc(1, "initialize", `{"protocolVersion":"2025-03-26","capabilities":{},"clientInfo":{"name":"smoke","version":"0"}}`) +
		rpc(0, "notifications/initialized", "") +
		rpc(2, "tools/list", `{}`) +
		rpc(3, "tools/call", fmt.Sprintf(`{"name":"search_trips","arguments":{"from":"Berlin","to":"Munich","date":%q,"max_results":3}}`, date))
	if _, err := io.WriteString(stdin, writes); err != nil {
		t.Fatalf("write requests: %v", err)
	}

	type toolsList struct {
		Result struct {
			Tools []struct {
				Name string `json:"name"`
			} `json:"tools"`
		} `json:"result"`
	}
	type callResult struct {
		Result struct {
			IsError           bool `json:"isError"`
			StructuredContent struct {
				FromCity   string `json:"from_city"`
				ToCity     string `json:"to_city"`
				Currency   string `json:"currency"`
				BookingURL string `json:"booking_url"`
				Trips      []struct {
					Departure string  `json:"departure"`
					Price     float64 `json:"price"`
				} `json:"trips"`
			} `json:"structuredContent"`
		} `json:"result"`
	}

	sc := bufio.NewScanner(stdout)
	sc.Buffer(make([]byte, 1<<20), 1<<20)
	deadline := time.AfterFunc(2*time.Minute, func() {
		if err := cmd.Process.Kill(); err != nil {
			t.Logf("deadline kill: %v", err)
		}
	})
	defer deadline.Stop()

	sawTools, sawTrips := false, false
	for (!sawTools || !sawTrips) && sc.Scan() {
		line := sc.Bytes()
		var probe struct {
			ID int `json:"id"`
		}
		if json.Unmarshal(line, &probe) != nil {
			continue
		}
		switch probe.ID {
		case 2:
			var tl toolsList
			if err := json.Unmarshal(line, &tl); err != nil {
				t.Fatalf("decode tools/list: %v", err)
			}
			names := make([]string, 0, len(tl.Result.Tools))
			for _, tool := range tl.Result.Tools {
				names = append(names, tool.Name)
			}
			got := strings.Join(names, ",")
			if !strings.Contains(got, "search_cities") || !strings.Contains(got, "search_trips") {
				t.Fatalf("tools/list = %q, want both tools", got)
			}
			sawTools = true
		case 3:
			var cr callResult
			if err := json.Unmarshal(line, &cr); err != nil {
				t.Fatalf("decode tools/call: %v", err)
			}
			out := cr.Result.StructuredContent
			if cr.Result.IsError {
				t.Fatalf("search_trips returned an error result: %s", line)
			}
			if out.FromCity != "Berlin" || out.ToCity != "Munich" {
				t.Errorf("route = %s → %s, want Berlin → Munich", out.FromCity, out.ToCity)
			}
			if len(out.Trips) == 0 {
				t.Fatal("no live trips returned")
			}
			if out.Trips[0].Price <= 0 {
				t.Errorf("first trip price = %v, want > 0", out.Trips[0].Price)
			}
			if !strings.Contains(out.BookingURL, "shop.flixbus.com") {
				t.Errorf("booking_url = %q", out.BookingURL)
			}
			sawTrips = true
		}
	}
	if err := sc.Err(); err != nil {
		t.Fatalf("read responses: %v", err)
	}
	if !sawTools || !sawTrips {
		t.Fatalf("missing responses: tools/list=%v tools/call=%v", sawTools, sawTrips)
	}

	// closing stdin ends the session cleanly: exit 0
	if err := stdin.Close(); err != nil {
		t.Fatalf("close stdin: %v", err)
	}
	if err := cmd.Wait(); err != nil {
		t.Errorf("stdio EOF shutdown: want exit 0, got %v", err)
	}
}

func TestHTTPServesWithStdinClosed(t *testing.T) {
	port := freePort(t)
	addr := fmt.Sprintf("127.0.0.1:%d", port)
	cmd := exec.CommandContext(t.Context(), binary, "-transport", "http", "-addr", addr) //nolint:gosec // runs the binary this test just built
	cmd.Stdin = nil                                                                      // stdin reads EOF immediately — the daemon must not care
	cmd.Stderr = io.Discard
	if err := cmd.Start(); err != nil {
		t.Fatalf("start: %v", err)
	}
	defer func() {
		if cmd.ProcessState == nil {
			if err := cmd.Process.Kill(); err != nil {
				t.Logf("kill: %v", err)
			}
			if err := cmd.Wait(); err != nil {
				t.Logf("wait after kill: %v", err)
			}
		}
	}()

	body := `{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2025-03-26","capabilities":{},"clientInfo":{"name":"smoke","version":"0"}}}`
	var resp *http.Response
	var err error
	for range 40 { // wait for the listener, up to ~10s
		req, reqErr := http.NewRequestWithContext(t.Context(), http.MethodPost, "http://"+addr+"/", strings.NewReader(body))
		if reqErr != nil {
			t.Fatalf("new request: %v", reqErr)
		}
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Accept", "application/json, text/event-stream")
		resp, err = http.DefaultClient.Do(req)
		if err == nil {
			break
		}
		time.Sleep(250 * time.Millisecond)
	}
	if err != nil {
		t.Fatalf("server never came up: %v", err)
	}
	defer func() {
		if cerr := resp.Body.Close(); cerr != nil {
			t.Logf("close body: %v", cerr)
		}
	}()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("initialize over http: status %d", resp.StatusCode)
	}

	// still running with stdin long closed, then SIGTERM must exit 0
	if err := cmd.Process.Signal(syscall.SIGTERM); err != nil {
		t.Fatalf("sigterm: %v", err)
	}
	if err := cmd.Wait(); err != nil {
		t.Errorf("SIGTERM shutdown: want exit 0, got %v", err)
	}
}

func freePort(t *testing.T) int {
	t.Helper()
	l, err := new(net.ListenConfig).Listen(t.Context(), "tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	tcpAddr, ok := l.Addr().(*net.TCPAddr)
	if !ok {
		t.Fatalf("unexpected addr type %T", l.Addr())
	}
	port := tcpAddr.Port
	if err := l.Close(); err != nil {
		t.Fatalf("close probe listener: %v", err)
	}
	return port
}
