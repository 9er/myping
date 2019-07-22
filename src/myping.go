package main

import (
    "github.com/sparrc/go-ping"
    ui "github.com/gizak/termui/v3"
    "github.com/gizak/termui/v3/widgets"
    "fmt"
    "time"
    "sync"
)

type Settings struct {
    Timeout time.Duration
    Interval time.Duration
    Count int
}

type Measurement struct {
    RTT time.Duration
    PacketsSent int
    PacketsRecv int
}

type Target struct {
    Address string
    Method int
    Measurements chan Measurement
}

const (
    ICMP = iota
    TCP
)

func probe_ping(measurements chan Measurement, wg *sync.WaitGroup, address string, settings *Settings) {
    defer wg.Done()

    for {
        pinger, err := ping.NewPinger(address)
        if err != nil {
            panic(err)
        }
        pinger.SetPrivileged(true)
        pinger.Count = settings.Count
        pinger.Interval = time.Duration(1)
        pinger.Timeout = time.Duration(settings.Timeout)
        pinger.Run()
        stats := pinger.Statistics()
        measurement := Measurement{
            RTT: stats.AvgRtt,
            PacketsSent: stats.PacketsSent,
            PacketsRecv: stats.PacketsRecv,
        }

        // call wg.Done() before the measurement is sent
        // because reading from the channel will block anyways
        wg.Done()
        measurements <- measurement
    }
}

func poll_targets(interval time.Duration, settings *Settings, targets []Target) {
    // start all measurements
    var wg sync.WaitGroup
    for _, target := range targets {
        if target.Method == ICMP {
            wg.Add(1)
            go probe_ping(target.Measurements , &wg, target.Address, settings)
        }
    }

    // periodically query the results (autostarts new measurements)
    for range time.Tick(interval) {
        // wait for all the measurements to finish
        wg.Wait()

        // FIXME DEBUG print results
        for _, target := range targets {
            wg.Add(1)
            measurement := <-target.Measurements
            if measurement.PacketsSent == measurement.PacketsRecv {
                fmt.Printf("!")
            } else if (measurement.PacketsRecv == 0) {
                fmt.Printf(".")
            } else {
                fmt.Printf("~")
            }
        }
	}
}

func poll_ui(stopped chan struct{}) {
	for e := range ui.PollEvents() {
		if e.Type == ui.KeyboardEvent {
            close(stopped)
		}
	}
}

func display_measurements() {
    p := widgets.NewParagraph()
	p.Text = "Hello World!"
	p.SetRect(0, 0, 25, 5)

	ui.Render(p)
}

func main() {
    // used to terminate gracefully
    stopped := make(chan struct{})

    // basic settings
    interval := time.Duration(time.Second)
    count := 5

    settings := Settings{}
    settings.Timeout = interval
    settings.Count = count

    // FIXME DEBUG
    var targets []Target
    testtarget := Target{
        Address: "129.143.2.1",
        Method: ICMP,
        Measurements: make(chan Measurement),
    }
    testtarget2 := Target{
        Address: "129.143.2.2",
        Method: ICMP,
        Measurements: make(chan Measurement),
    }
    targets = append(targets, testtarget)
    targets = append(targets, testtarget2)

	// start the UI
	if err := ui.Init(); err != nil {
		fmt.Printf("failed to initialize termui: %v", err)
	}
	defer ui.Close()

    // enter the polling main loop
    go poll_targets(time.Duration(time.Second), &settings, targets)
	go poll_ui(stopped)

    select {
        case <-stopped:
            break
    }
}
