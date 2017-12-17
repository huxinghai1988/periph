// Copyright 2017 The Periph Authors. All rights reserved.
// Use of this source code is governed under the Apache License, Version 2.0
// that can be found in the LICENSE file.

// gpio-web runs a web based digital pin (GPIO) viewer.
package main

import (
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"net"
	"net/http"
	"os"
	"os/signal"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"time"

	"periph.io/x/periph"
	"periph.io/x/periph/conn/gpio"
	"periph.io/x/periph/conn/gpio/gpioreg"
	"periph.io/x/periph/conn/gpio/gpiostream"
	"periph.io/x/periph/conn/gpio/gpiotest"
	"periph.io/x/periph/host"
	"periph.io/x/periph/host/bcm283x"
)

// readSamplesBCM reads GPIOs at the specified frequency via the bcm283x.
func readSamplesBCM(b *gpiostream.BitStreamLSB, pins []string) error {
	if b.Res%time.Microsecond != 0 {
		return fmt.Errorf("resolution %s must be rounded to 1µs", b.Res)
	}
	if len(pins) > 9 {
		return errors.New("maximum 8 pins can be read simultaneously")
	}

	var num []int
	for _, p := range pins {
		if strings.HasPrefix(p, "GPIO") {
			n, err := strconv.Atoi(p[4:])
			if err != nil {
				return fmt.Errorf("failed to process %s: %v", p, err)
			}
			num = append(num, n)
		} else {
			// Fallback into the generic code path.
			return readSamples(b, pins)
		}
	}

	var masks [8]uint32
	var shifts [8]uint32
	for i, p := range num {
		if p < 0 || p > 31 {
			return errors.New("invalid pin, supported range is [0, 31]")
		}
		masks[i] = 1 << uint(p)
		shifts[i] = uint32(p - i)
	}
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	delta := b.Res
	next := bcm283x.ReadTime() + delta
	bits := b.Bits
	// Acceptable jitter in time to execute the read operation. We use 1%.
	acceptable := delta / 100
	for i := range bits {
		// Use a busy loop to reduce jitter.
		for {
			t := bcm283x.ReadTime()
			if t >= next && t <= next+acceptable {
				next += delta
				break
			}
			if t > next {
				// TODO(maruel): If less than 0.5%, probably not a big deal, e.g. 5µs
				// when reading at 1ms. Can't log because it'll increase jitter even
				// more.
				return fmt.Errorf("overrun by %s", t-next)
			}
		}
		// The following must take less than 1µs. It must not make function calls
		// nor allocate on the heap.
		v := bcm283x.PinsRead0To31()
		x := (v & masks[0]) >> shifts[0]
		x |= (v & masks[1]) >> shifts[1]
		x |= (v & masks[2]) >> shifts[2]
		x |= (v & masks[3]) >> shifts[3]
		x |= (v & masks[4]) >> shifts[4]
		x |= (v & masks[5]) >> shifts[5]
		x |= (v & masks[6]) >> shifts[6]
		x |= (v & masks[7]) >> shifts[7]
		bits[i] = byte(x)
	}
	return nil
}

// readSamples reads arbitrary GPIOs at the specified frequency.
func readSamples(b *gpiostream.BitStreamLSB, pins []string) error {
	if b.Res%(10*time.Microsecond) != 0 {
		return fmt.Errorf("resolution %s must be rounded to 10µs", b.Res)
	}
	if len(pins) > 2 {
		return errors.New("maximum 2 pins can be read simultaneously in the general code path")
	}
	var p [2]gpio.PinIn
	for i, pin := range pins {
		if p[i] = gpioreg.ByName(pin); p[i] == nil {
			return fmt.Errorf("pins %q is unavailable", pin)
		}
	}

	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	delta := b.Res
	next := time.Now().Add(delta)
	bits := b.Bits
	// Acceptable jitter in time to execute the read operation. We use 1%.
	acceptable := delta / 100
	for i := range bits {
		// Use a busy loop to reduce jitter.
		for {
			now := time.Now()
			if now.Equal(next) {
				next = next.Add(delta)
				break
			}
			if now.After(next) {
				if now.After(next.Add(acceptable)) {
					return fmt.Errorf("overrun by %s", now.Sub(next))
				}
				next = next.Add(delta)
				break
			}
		}
		x := byte(0)
		if p[0].Read() {
			x = 1
		}
		if p[1] != nil && p[1].Read() {
			x |= 2
		}
		bits[i] = x
	}
	return nil
}

/*
// readSamplesFake returns fake data for testing locally.
func readSamplesFake(b *gpiostream.BitStreamLSB, pins []string) error {
	bits := b.Bits
	for i := 0; i < len(bits); i++ {
		mask := byte(0)
		for j, p := range pins {
			switch p {
			case "HIGH":
				mask |= 1 << uint(j)
			case "SQUARE":
				if i&1 != 0 {
					mask |= 1 << uint(j)
				}
			default:
			}
		}
		bits[i] = mask
	}
	return nil
}
*/

//

const cacheControl30d = "Cache-Control:public,max-age=259200" // 30d
const cacheControl5m = "Cache-Control:public,max-age=300"     // 5m
const cacheControlNone = "Cache-Control:no-cache,private"

type webServer struct {
	ln       net.Listener
	server   http.Server
	state    *periph.State
	hostname string
	isBCM    bool

	mu sync.Mutex
	//fake bool
	pins []gpio.PinIO
}

func (s *webServer) rootHandler(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/" {
		http.Error(w, "Not Found", http.StatusNotFound)
		return
	}
	if r.Method != "GET" {
		http.Error(w, "Only GET is allowed", http.StatusMethodNotAllowed)
		return
	}
	w.Header().Set("Content-Type", "text/html")
	w.Header().Set("Cache-Control", cacheControl5m)
	keys := map[string]interface{}{
		"Hostname": s.hostname,
		"State":    s.state,
		// TODO(maruel): Remove.
		//"Bits":     make([]struct{}, 32),
	}
	/*
		if s.isBCM {
			keys["Bits"] = bcm283x.PinsRead0To31()
		}
	*/
	if err := rootTpl.Execute(w, keys); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}
}

func (s *webServer) faviconHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != "GET" {
		http.Error(w, "Only GET is allowed", http.StatusMethodNotAllowed)
		return
	}
	w.Header().Set("Content-Type", "image/png")
	w.Header().Set("Cache-Control", cacheControl30d)
	w.Write(favicon)
}

func (s *webServer) readHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != "GET" {
		http.Error(w, "Only GET is allowed", http.StatusMethodNotAllowed)
		return
	}
	v := r.FormValue("pins")
	if v == "" {
		http.Error(w, "query argument \"pins\" is required", 400)
		return
	}
	pins := strings.Split(v, ",")

	v = r.FormValue("res")
	if v == "" {
		http.Error(w, "query argument \"res\" is required", 400)
		return
	}
	res, err := time.ParseDuration(v)
	if err != nil {
		http.Error(w, "failed to parse \"res\"", 400)
		return
	}

	v = r.FormValue("dur")
	if v == "" {
		http.Error(w, "query argument \"dur\" is required", 400)
		return
	}
	dur, err := time.ParseDuration(v)
	if err != nil {
		http.Error(w, "failed to parse \"dur\"", 400)
		return
	}
	if dur%res != 0 {
		http.Error(w, "\"dur\" must be a multiple of \"res\"", 400)
		return
	}

	// TODO(maruel): Stream out, and call http.Flusher every 512~1024 bytes,
	// based on graph width.
	b := gpiostream.BitStreamLSB{Res: res, Bits: make(gpiostream.BitsLSB, dur/res)}
	/*if s.fake {
		// There's no pin on the host.
		err = readSamplesFake(&b, pins)
	} else*/if s.isBCM {
		err = readSamplesBCM(&b, pins)
	} else {
		err = readSamples(&b, pins)
	}
	if err != nil {
		http.Error(w, err.Error(), 400)
		return
	}
	w.Header().Set("Content-Type", "application/octet-stream")
	w.Header().Set("Cache-Control", cacheControlNone)
	w.Write(b.Bits)
}

func (s *webServer) allHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != "GET" {
		http.Error(w, "Only GET is allowed", http.StatusMethodNotAllowed)
		return
	}
	// TODO(maruel): Skip uninteresting pins.
	data := map[string]int{}
	for _, p := range s.pins {
		data[p.Name()] = 0
		if p.Read() {
			data[p.Name()] = 1
		}
	}
	d, err := json.Marshal(data)
	if err != nil {
		http.Error(w, err.Error(), 500)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", cacheControlNone)
	w.Write(d)
}

func (s *webServer) Close() error {
	return s.ln.Close()
}

type square struct {
	gpiotest.Pin
}

func (s *square) Read() gpio.Level {
	s.Lock()
	l := s.L
	s.L = !s.L
	s.Unlock()
	return l
}

func newWebServer(port string, state *periph.State) (*webServer, error) {
	s := &webServer{
		state: state,
		isBCM: bcm283x.Present(),
		pins:  gpioreg.All(),
	}
	if len(s.pins) == 0 {
		//s.fake = true
		s.pins = []gpio.PinIO{
			&square{gpiotest.Pin{N: "SQUARE"}},
			&gpiotest.Pin{N: "HIGH", Num: 1, L: gpio.High},
			&gpiotest.Pin{N: "LOW", Num: 2},
		}
		for _, p := range s.pins {
			if err := gpioreg.Register(p, true); err != nil {
				return nil, err
			}
		}
	}
	var err error
	if s.hostname, err = os.Hostname(); err != nil {
		return nil, err
	}
	http.HandleFunc("/all", s.allHandler)
	http.HandleFunc("/read", s.readHandler)
	http.HandleFunc("/favicon.ico", s.faviconHandler)
	http.HandleFunc("/", s.rootHandler)
	if s.ln, err = net.Listen("tcp", port); err != nil {
		return nil, err
	}
	s.server = http.Server{
		Addr:           s.ln.Addr().String(),
		Handler:        http.DefaultServeMux,
		ReadTimeout:    365 * 24 * time.Hour,
		WriteTimeout:   60 * time.Second,
		MaxHeaderBytes: 1 << 16,
	}
	go s.server.Serve(s.ln)
	return s, nil
}

func mainImpl() error {
	port := flag.String("http", ":80", "IP and port to bind to")
	flag.Parse()
	if flag.NArg() != 0 {
		return errors.New("unsupported arguments")
	}
	state, err := host.Init()
	if err != nil {
		return err
	}
	s, err := newWebServer(*port, state)
	if err != nil {
		return err
	}
	c := make(chan os.Signal)
	go func() { <-c }()
	signal.Notify(c, os.Interrupt)
	<-c
	s.Close()
	return nil
}

func main() {
	if err := mainImpl(); err != nil {
		fmt.Fprintf(os.Stderr, "gpio-web: %s.\n", err)
		os.Exit(1)
	}
}
