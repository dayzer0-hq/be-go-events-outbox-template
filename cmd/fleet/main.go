// Command fleet runs N virtual scooters with configurable offline periods, a synchronised
// reconnection surge, devices replaying a day of buffered frames, and a malformed-frame generator
// — the device simulator the Ather brief promises.
//
// ⚠️ IT SENDS; IT DOES NOT INGEST. Batching writes, keeping per-device ordering, applying
// backpressure and routing a bad frame to the dead-letter path are the project. This produces the
// traffic those decisions have to survive, including the frames that should be refused.
package main

import (
	"bytes"
	"encoding/json"
	"flag"
	"fmt"
	"math/rand"
	"net/http"
	"os"
	"sync"
	"time"
)

type frame struct {
	DeviceID string  `json:"device_id"`
	Seq      int     `json:"seq"`
	At       string  `json:"at"`
	SoC      float64 `json:"soc_percent"`
	Speed    float64 `json:"speed_kmph"`
	Odo      float64 `json:"odometer_km"`
	Firmware string  `json:"firmware"`
}

func main() {
	base := flag.String("base", "http://localhost:8080", "base URL of your ingest")
	path := flag.String("path", "/telemetry", "ingest endpoint")
	devices := flag.Int("devices", 20, "number of virtual scooters")
	frames := flag.Int("frames", 25, "frames per device")
	offline := flag.Float64("offline", 0.0, "fraction of devices that go offline and buffer (0.0 to 1.0)")
	surge := flag.Bool("surge", false, "all offline devices reconnect at the SAME instant and flush together")
	malformed := flag.Float64("malformed", 0.0, "fraction of frames that are deliberately bad")
	flag.Usage = func() {
		fmt.Fprintf(os.Stderr, "fleet — N virtual scooters sending telemetry.\n\n"+
			"  -offline F   that fraction buffer their frames instead of sending live\n"+
			"  -surge       every buffered device flushes at ONE instant — the reconnection surge\n"+
			"  -malformed F that fraction of frames are bad, in four ways:\n"+
			"                 truncated payload · impossible value (SoC 900%%) ·\n"+
			"                 unknown firmware · a device whose CLOCK IS WRONG (timestamps in 2019)\n\n"+
			"The wrong-clock device is the interesting one: every frame is well-formed and the only\n"+
			"thing wrong is the time, so ordering by the device's own timestamp puts it first for ever.\n\nFlags:\n")
		flag.PrintDefaults()
	}
	flag.Parse()

	client := &http.Client{Timeout: 15 * time.Second}
	var mu sync.Mutex
	codes := map[int]int{}
	errs := 0
	send := func(f frame, bad string) {
		var body []byte
		switch bad {
		case "truncated":
			b, _ := json.Marshal(f)
			body = b[:len(b)/2]
		case "impossible":
			f.SoC = 900
			f.Speed = -40
			body, _ = json.Marshal(f)
		case "firmware":
			f.Firmware = "9.9.9-unreleased"
			body, _ = json.Marshal(f)
		case "clock":
			f.At = time.Date(2019, 1, 1, 0, 0, 0, 0, time.UTC).Format(time.RFC3339)
			body, _ = json.Marshal(f)
		default:
			body, _ = json.Marshal(f)
		}
		resp, err := client.Post(*base+*path, "application/json", bytes.NewReader(body))
		mu.Lock()
		defer mu.Unlock()
		if err != nil {
			errs++
			return
		}
		resp.Body.Close()
		codes[resp.StatusCode]++
	}

	badKinds := []string{"truncated", "impossible", "firmware", "clock"}
	gate := make(chan struct{})
	var wg sync.WaitGroup
	buffered := 0
	for d := 0; d < *devices; d++ {
		isOffline := rand.Float64() < *offline
		if isOffline {
			buffered++
		}
		wg.Add(1)
		go func(d int, isOffline bool) {
			defer wg.Done()
			id := fmt.Sprintf("scooter-%04d", d)
			batch := make([]frame, 0, *frames)
			at := time.Now().Add(-time.Duration(*frames) * time.Second)
			for s := 0; s < *frames; s++ {
				at = at.Add(time.Second)
				batch = append(batch, frame{id, s, at.Format(time.RFC3339), 100 - float64(s)*0.4,
					float64(20 + rand.Intn(45)), float64(s) * 0.15, "2.4.1"})
			}
			if isOffline && *surge {
				<-gate // every buffered device flushes together
			}
			for _, f := range batch {
				bad := ""
				if rand.Float64() < *malformed {
					bad = badKinds[rand.Intn(len(badKinds))]
				}
				send(f, bad)
				if !isOffline {
					time.Sleep(time.Duration(20+rand.Intn(60)) * time.Millisecond)
				}
			}
		}(d, isOffline)
	}
	if *surge {
		fmt.Printf("%d device(s) buffered; releasing the surge in 1s…\n", buffered)
		time.Sleep(time.Second)
	}
	close(gate)
	wg.Wait()

	fmt.Printf("\nfleet — %d devices × %d frames\n", *devices, *frames)
	for c, n := range codes {
		fmt.Printf("  HTTP %d  %d\n", c, n)
	}
	if errs > 0 {
		fmt.Printf("  transport errors  %d\n", errs)
	}
	if *malformed > 0 {
		fmt.Println("\nA bad frame should be refused or dead-lettered, never silently stored and never\n" +
			"enough to stop the good frames behind it. Run cmd/verify to see which happened.")
	}
}
