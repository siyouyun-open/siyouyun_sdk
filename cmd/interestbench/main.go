// interestbench: end-to-end benchmark for v1 SDK target mode.
// Creates multiple app listeners (one FileEventMonitor per app), writes files, and counts handler callbacks.
package main

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/nats-io/nats.go"

	"github.com/siyouyun-open/siyouyun_sdk/ability"
	sdkconst "github.com/siyouyun-open/siyouyun_sdk/pkg/const"
	sdkdto "github.com/siyouyun-open/siyouyun_sdk/pkg/dto"
	"github.com/siyouyun-open/siyouyun_sdk/pkg/restclient"
)

func main() {
	apps := flag.Int("apps", 10, "number of listeners/apps")
	files := flag.Int("files", 1000, "number of files to write")
	dir := flag.String("dir", "/home/admin/main/syspool/Photos", "real directory")
	ufiDir := flag.String("ufi", "/Photos", "OS watched dir (relative/UFI)")
	media := flag.String("media", "image", "mediaType")
	exclude := flag.String("exclude", ".tmp,.part", "excluded extensions")
	action := flag.String("action", "add", "add/delete")
	timeout := flag.Duration("timeout", 120*time.Second, "timeout waiting for all callbacks")
	flag.Parse()

	restclient.InitHttpClient()
	nc, err := nats.Connect(nats.DefaultURL)
	if err != nil {
		panic(err)
	}
	defer nc.Close()
	if err := os.MkdirAll(*dir, 0o755); err != nil {
		panic(err)
	}
	// Clean old files to avoid duplicate add events.
	if err := filepath.Walk(*dir, func(path string, info os.FileInfo, err error) error {
		if err == nil && strings.HasPrefix(info.Name(), "sdkbench_") {
			_ = os.Remove(path)
		}
		return nil
	}); err != nil {
		panic(err)
	}

	eventType := sdkconst.FileEventAdd
	switch *action {
	case "add":
	case "delete":
		eventType = sdkconst.FileEventDelete
	default:
		panic("action must be add or delete")
	}
	if eventType == sdkconst.FileEventDelete {
		// Delete bench requires files to exist before the delete listener starts.
		for k := 0; k < *files; k++ {
			name := filepath.Join(*dir, fmt.Sprintf("sdkbench_%d.jpg", k))
			if err := os.WriteFile(name, payloadForBench(), 0o644); err != nil {
				panic(err)
			}
		}
	}

	counts := make([]atomic.Int64, *apps)
	writeAt := make([]atomic.Int64, *files)
	var latMu sync.Mutex
	latencies := make([]int64, 0, *files)
	monitors := make([]*ability.FileEventMonitor, 0, *apps)
	for i := 0; i < *apps; i++ {
		appCode := fmt.Sprintf("benchapp%d", i)
		idx := i
		mon := ability.NewFileEventMonitor(&appCode, nc)
		_, err := mon.StartFileEventListener(ability.FileEventListenerOptions{
			Dirs:        []string{*ufiDir},
			MediaTypes:  []sdkconst.MediaType{sdkconst.MediaType(*media)},
			Actions:     []int{eventType},
			ExcludeExts: splitExts(*exclude),
			QoS:         sdkconst.MidLevel,
		}, func(fe *sdkdto.FileEvent) (sdkconst.ConsumeStatus, string) {
			counts[idx].Add(1)
			if idx == 0 {
				if k, ok := benchIndex(fe.UFI); ok && k < len(writeAt) {
					if w := writeAt[k].Load(); w > 0 {
						latMu.Lock()
						latencies = append(latencies, time.Now().UnixNano()-w)
						latMu.Unlock()
					}
				}
			}
			return sdkconst.ConsumeSuccess, ""
		})
		if err != nil {
			panic(err)
		}
		monitors = append(monitors, mon)
	}
	defer func() {
		for _, m := range monitors {
			m.Close()
		}
	}()
	time.Sleep(2 * time.Second) // wait for the Fetch loop to become ready

	payload := payloadForBench()
	start := time.Now()
	for k := 0; k < *files; k++ {
		name := filepath.Join(*dir, fmt.Sprintf("sdkbench_%d.jpg", k))
		writeAt[k].Store(time.Now().UnixNano())
		if eventType == sdkconst.FileEventDelete {
			if err := os.Remove(name); err != nil {
				panic(err)
			}
		} else if err := os.WriteFile(name, payload, 0o644); err != nil {
			panic(err)
		}
	}
	gen := time.Since(start)

	want := int64(*apps * *files)
	deadline := time.Now().Add(*timeout)
	for time.Now().Before(deadline) {
		var total int64
		for i := range counts {
			total += counts[i].Load()
		}
		if total >= want {
			break
		}
		time.Sleep(20 * time.Millisecond)
	}
	var total int64
	for i := range counts {
		total += counts[i].Load()
	}
	elapsed := time.Since(start)
	latMu.Lock()
	sort.Slice(latencies, func(i, j int) bool { return latencies[i] < latencies[j] })
	p50, p95, p99, max := percentile(latencies, 50), percentile(latencies, 95), percentile(latencies, 99), percentile(latencies, 100)
	nlat := len(latencies)
	latMu.Unlock()
	fmt.Printf("SDKBENCH apps=%d files=%d want=%d got=%d ratio=%.2f gen_ms=%d total_ms=%d rate_ev_s=%.1f lat_n=%d p50_ms=%.1f p95_ms=%.1f p99_ms=%.1f max_ms=%.1f\n",
		*apps, *files, want, total, float64(total)*100/float64(want), gen.Milliseconds(), elapsed.Milliseconds(),
		float64(total)*1000/float64(elapsed.Milliseconds()+1), nlat,
		float64(p50)/1e6, float64(p95)/1e6, float64(p99)/1e6, float64(max)/1e6)
	_ = filepath.Walk(*dir, func(path string, info os.FileInfo, err error) error {
		if err == nil && strings.HasPrefix(info.Name(), "sdkbench_") {
			_ = os.Remove(path)
		}
		return nil
	})
}

func payloadForBench() []byte { return []byte(strings.Repeat("x", 1024)) }

func benchIndex(ufi string) (int, bool) {
	base := filepath.Base(ufi)
	base = strings.TrimSuffix(base, ".jpg")
	const prefix = "sdkbench_"
	if !strings.HasPrefix(base, prefix) {
		return 0, false
	}
	k, err := strconv.Atoi(strings.TrimPrefix(base, prefix))
	return k, err == nil
}

func percentile(sorted []int64, p int) int64 {
	if len(sorted) == 0 {
		return 0
	}
	i := (len(sorted) - 1) * p / 100
	return sorted[i]
}

func splitExts(s string) []string {
	var out []string
	for _, p := range strings.Split(s, ",") {
		p = strings.TrimSpace(p)
		if p != "" {
			out = append(out, p)
		}
	}
	return out
}
