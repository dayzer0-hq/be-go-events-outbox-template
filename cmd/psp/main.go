// Command psp runs four payment-provider stubs with independently configurable latency, failure
// rate and timeout behaviour — including a "charged but did not answer" mode — plus a status API
// you can query later to find out what really happened. The stubs the Juspay brief promises.
//
// ⚠️ THESE ARE THE PROVIDERS, NOT YOUR ORCHESTRATOR. Routing between them, tripping a breaker and
// resolving a timeout that might have gone through are the project. The point of the
// charged-but-silent mode is that the ONLY way to find out is to ask the status API afterwards —
// which is exactly the reconciliation your service has to do.
package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"math/rand"
	"net/http"
	"os"
	"strings"
	"sync"
	"time"
)

type charge struct {
	ID       string    `json:"id"`
	Provider string    `json:"provider"`
	Amount   int       `json:"amount_paise"`
	State    string    `json:"state"` // CHARGED | FAILED
	At       time.Time `json:"at"`
	Answered bool      `json:"answered"` // did the caller ever get a reply?
}

type provider struct {
	name      string
	latency   time.Duration
	failRate  float64
	timeout   float64 // fraction of calls that never answer
	silentPay float64 // fraction that CHARGE and then never answer
}

var (
	mu      sync.Mutex
	charges = map[string]*charge{}
)

func main() {
	listen := flag.String("listen", ":9092", "address to serve on")
	latency := flag.Duration("latency", 120*time.Millisecond, "base latency for every provider")
	failRate := flag.Float64("fail-rate", 0.05, "fraction of calls that fail cleanly (0.0 to 1.0)")
	timeoutRate := flag.Float64("timeout-rate", 0.0, "fraction that never answer at all")
	silentRate := flag.Float64("charged-but-silent", 0.0, "fraction that CHARGE and then never answer")
	flag.Usage = func() {
		fmt.Fprintf(os.Stderr, "psp — four payment providers with configurable behaviour.\n\n"+
			"  POST /{provider}/charge   razorpay | payu | cashfree | billdesk\n"+
			"  GET  /{provider}/status/{id}   what really happened to that charge\n"+
			"  GET  /healthz\n\n"+
			"⚠️ -charged-but-silent is the mode the brief is about: the money moves and the caller\n"+
			"never hears. Your orchestrator cannot tell it from -timeout-rate at the time; the only\n"+
			"difference is what the status API says afterwards.\n\n"+
			"Per-provider overrides: -payu-fail-rate, -cashfree-latency and so on are NOT supported;\n"+
			"run a second copy on another port when you need providers to differ.\n\nFlags:\n")
		flag.PrintDefaults()
	}
	flag.Parse()

	provs := map[string]*provider{}
	for i, n := range []string{"razorpay", "payu", "cashfree", "billdesk"} {
		// Staggered latency so routing has something to prefer, without any of them being "right".
		provs[n] = &provider{n, *latency + time.Duration(i)*40*time.Millisecond, *failRate, *timeoutRate, *silentRate}
	}

	mux := http.NewServeMux()
	mux.HandleFunc("GET /healthz", func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"status":"ok"}`))
	})
	mux.HandleFunc("POST /{provider}/charge", func(w http.ResponseWriter, r *http.Request) {
		p, ok := provs[strings.ToLower(r.PathValue("provider"))]
		if !ok {
			http.Error(w, `{"error":"unknown_provider"}`, http.StatusNotFound)
			return
		}
		var in struct {
			ID     string `json:"id"`
			Amount int    `json:"amount_paise"`
		}
		_ = json.NewDecoder(r.Body).Decode(&in)
		if in.ID == "" {
			in.ID = fmt.Sprintf("chg-%d", time.Now().UnixNano())
		}
		time.Sleep(p.latency + time.Duration(rand.Int63n(int64(p.latency))))

		roll := rand.Float64()
		switch {
		case roll < p.silentPay:
			// Charged, then silence. This is the whole point of the brief.
			record(in.ID, p.name, in.Amount, "CHARGED", false)
			log.Printf("psp %s: CHARGED %s and did NOT answer", p.name, in.ID)
			hijackAndHang(w)
			return
		case roll < p.silentPay+p.timeout:
			log.Printf("psp %s: hanging on %s without charging", p.name, in.ID)
			record(in.ID, p.name, in.Amount, "FAILED", false)
			hijackAndHang(w)
			return
		case roll < p.silentPay+p.timeout+p.failRate:
			record(in.ID, p.name, in.Amount, "FAILED", true)
			w.WriteHeader(http.StatusBadGateway)
			_ = json.NewEncoder(w).Encode(map[string]any{"id": in.ID, "state": "FAILED", "provider": p.name})
			return
		}
		record(in.ID, p.name, in.Amount, "CHARGED", true)
		_ = json.NewEncoder(w).Encode(map[string]any{"id": in.ID, "state": "CHARGED", "provider": p.name})
	})
	mux.HandleFunc("GET /{provider}/status/{id}", func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		defer mu.Unlock()
		c, ok := charges[r.PathValue("id")]
		if !ok {
			http.Error(w, `{"error":"unknown_charge"}`, http.StatusNotFound)
			return
		}
		_ = json.NewEncoder(w).Encode(c)
	})

	fmt.Printf("psp listening on %s — providers: razorpay payu cashfree billdesk\n", *listen)
	log.Fatal(http.ListenAndServe(*listen, mux))
}

func record(id, prov string, amt int, state string, answered bool) {
	mu.Lock()
	defer mu.Unlock()
	charges[id] = &charge{ID: id, Provider: prov, Amount: amt, State: state, At: time.Now(), Answered: answered}
}

// hijackAndHang drops the connection without a reply, which is what a provider that never answers
// looks like from the caller's side — distinct from a 5xx, which is an answer.
func hijackAndHang(w http.ResponseWriter) {
	hj, ok := w.(http.Hijacker)
	if !ok {
		time.Sleep(60 * time.Second)
		return
	}
	conn, _, err := hj.Hijack()
	if err != nil {
		return
	}
	go func() { time.Sleep(60 * time.Second); conn.Close() }()
}
