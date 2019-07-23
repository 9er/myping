package main

import (
    "github.com/sparrc/go-ping"
    "fmt"
    "time"
    "sync"
    "io"
    "os"
)

type Settings struct {
    Timeout time.Duration
    Interval time.Duration
    PacketInterval time.Duration
    Count int
}

type Measurement struct {
    RTT time.Duration
    PacketsSent int
    PacketsRecv int
}

type Target struct {
    sync.Mutex
    Address string
    Data []Measurement
}

const (
    ICMP = iota
    TCP
)

func probeTarget(wg *sync.WaitGroup, target *Target, settings *Settings) {
    defer wg.Done()

    target.Lock()
    pinger, err := ping.NewPinger(target.Address)
    if err != nil {
        panic(err)
    }
    pinger.SetPrivileged(true)
    pinger.Count = settings.Count
    pinger.Interval = settings.PacketInterval
    pinger.Timeout = time.Duration(settings.Timeout)

    pinger.Run()

    stats := pinger.Statistics()
    measurement := Measurement{
        RTT: stats.AvgRtt,
        PacketsSent: stats.PacketsSent,
        PacketsRecv: stats.PacketsRecv,
    }

    target.Data = append(target.Data, measurement)
    target.Unlock()
}

func updateLoop(targets []*Target, uiupdate chan struct{}, settings *Settings) {
    // periodically start new measurements
    ticker := time.NewTicker(settings.Interval)
    for range ticker.C {
        wg := sync.WaitGroup{}
        for _, target := range targets {
            wg.Add(1)
            go probeTarget(&wg, target, settings)
        }
        wg.Wait()
        uiupdate <- struct{}{}
	}
}

func buildLine(description string, data []Measurement, maxdesc int, maxwidth int) string {
    spacing := 2

    var desccolor string
    lastmeasurement := data[len(data)-1]
    if lastmeasurement.PacketsSent == lastmeasurement.PacketsRecv {
        desccolor = ""
    } else if lastmeasurement.PacketsRecv == 0 {
        desccolor = "\033[31m"
    } else {
        desccolor = "\033[33m"
    }

    descpart := fmt.Sprintf(fmt.Sprintf("%%-%ds", maxdesc), description)

    datalen := maxwidth - maxdesc - spacing
    var trimmeddata []Measurement
    if len(data) > datalen {
        trimmeddata = data[len(data)-datalen:]
    } else {
        trimmeddata = data
    }

    var vispart string
    if len(trimmeddata) < datalen {
        // fill from the left with empty spaces
        vispart = fmt.Sprintf(fmt.Sprintf("%%%ds", datalen - len(trimmeddata)), "")
    }
    for _, measurement := range trimmeddata {
        //if measurement.PacketsSent == measurement.PacketsRecv {
        //    vispart = vispart + "\033[32m!\033[0m"
        //} else if measurement.PacketsRecv == 0 {
        //    vispart = vispart + "\033[31m.\033[0m"
        //} else {
        //    vispart = vispart + "\033[33m~\033[0m"
        //}
        if measurement.PacketsSent == measurement.PacketsRecv {
            if measurement.PacketsSent == 0 {
                vispart = vispart + " "
            } else {
                vispart = vispart + "\033[32m█\033[0m"
            }
        } else if measurement.PacketsRecv == 0 {
            vispart = vispart + "\033[41m \033[0m"
        } else {
            vispart = vispart + "\033[43m \033[0m"
        }
    }
    // example output: "%s%2s%s"
    formatstring := fmt.Sprintf("%%s%%s\033[0m%%%ds%%s\n", spacing)
    return fmt.Sprintf(formatstring, desccolor, descpart, "", vispart)
}

func displayStats(uiupdate chan struct{}, targets []*Target) {
    for range uiupdate {
        // was: for { <- uiupdate
        for index, target := range targets {
            target.Lock()
            line := buildLine(target.Address, target.Data, 14, 80)
            target.Unlock()
            num := len(targets) - index
            // move cursor to the correct line
            io.WriteString(os.Stdout, fmt.Sprintf("\033[%dA", num))
            io.WriteString(os.Stdout, line)
            // reset cursor
            io.WriteString(os.Stdout, "\0338")
        }
    }
}

func makeTarget(address string, settings *Settings) *Target {
    target := Target{
        Address: address,
    }
    return &target
}

func main() {
    // used to terminate gracefully
    stopped := make(chan struct{})

    // basic settings
    interval := time.Second
    count := 5

    settings := Settings{}
    settings.Interval = interval
    settings.PacketInterval = time.Duration(interval / time.Duration(2 * count))
    settings.Timeout = time.Duration(interval / 2)
    settings.Count = count

    // FIXME DEBUG
    var targets []*Target
    targets = append(targets, makeTarget("129.143.2.1", &settings))
    targets = append(targets, makeTarget("129.143.4.2", &settings))
    targets = append(targets, makeTarget("8.8.8.8", &settings))
    targets = append(targets, makeTarget("1.1.1.1", &settings))

    for range targets {
        fmt.Println("")
    }
    io.WriteString(os.Stdout, "\0337")
    uiupdate := make(chan struct{})

    // enter the polling main loop
    go updateLoop(targets, uiupdate, &settings)
    go displayStats(uiupdate, targets)

    //displayList(targets)

    select {
        case <-stopped:
            break
    }
}
