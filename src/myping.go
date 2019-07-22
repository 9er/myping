package main

import (
    "github.com/sparrc/go-ping"
    ui "github.com/gizak/termui/v3"
    "github.com/gizak/termui/v3/widgets"
    "fmt"
    "time"
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
    ProbeResults chan Measurement
    Data []Measurement
}

const (
    ICMP = iota
    TCP
)

func probeICMP(address string, settings *Settings) Measurement {
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

    return measurement
}

func probeTarget(target *Target, settings *Settings) {
    var result Measurement
    if target.Method == ICMP {
        result = probeICMP(target.Address, settings)
    }
    target.ProbeResults <- result
}

func updateLoop(interval time.Duration, targets []*Target, settings *Settings) {
    // start all measurements
    for _, target := range targets {
        go probeTarget(target, settings)
    }

    // periodically query the results (autostarts new measurements)
    for range time.Tick(interval) {
        // FIXME DEBUG print results
        for _, target := range targets {
            measurement := <-target.ProbeResults
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

func pollUiEvents(stopped chan struct{}) {
	for e := range ui.PollEvents() {
		if e.Type == ui.KeyboardEvent {
            close(stopped)
		}
	}
}

func displayList(targets []Target) {
    list := widgets.NewList()
    list.WrapText = false
	list.Title = "Targets"
    for _, target := range targets {
        list.Rows = append(list.Rows, target.Address)
    }
    list.SetRect(0, 0, 25, 8)

	ui.Render(list)
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
    var targets []*Target
    testtarget := Target{
        Address: "129.143.2.1",
        Method: ICMP,
        ProbeResults: make(chan Measurement),
    }
    testtarget2 := Target{
        Address: "129.143.2.2",
        Method: ICMP,
        ProbeResults: make(chan Measurement),
    }
    targets = append(targets, &testtarget)
    targets = append(targets, &testtarget2)

	// start the UI
	//if err := ui.Init(); err != nil {
	//	fmt.Printf("failed to initialize termui: %v", err)
	//}
	//defer ui.Close()

    // enter the polling main loop
    go updateLoop(time.Duration(time.Second), targets, &settings)
	go pollUiEvents(stopped)

    //displayList(targets)

    select {
        case <-stopped:
            break
    }
}
