package main

import (
    "github.com/sparrc/go-ping"
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
        measurements <- measurement
        wg.Done()
    }
}

func mainloop(interval time.Duration, settings *Settings, targets []Target) {
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
        fmt.Println("all done")

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

func main() {
    interval := time.Duration(time.Second)
    count := 5

    settings := Settings{}
    settings.Timeout = interval
    settings.Count = count

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

    mainloop(time.Duration(time.Second), &settings, targets)
}
