package main

import (
    "github.com/sparrc/go-ping"
    "fmt"
    "flag"
    "time"
    "sync"
    "io"
    "os"
    "syscall"
    "unsafe"
)

type winsize struct {
    Row    uint16
    Col    uint16
    Xpixel uint16
    Ypixel uint16
}

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

func getWidth() int {
    ws := &winsize{}
    ret, _, err := syscall.Syscall(syscall.SYS_IOCTL, uintptr(syscall.Stdin), uintptr(syscall.TIOCGWINSZ), uintptr(unsafe.Pointer(ws)))

    if int(ret) == -1 {
        panic(err)
    }
    return int(ws.Col)
}

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
    if len(data) > 0 {
        lastmeasurement := data[len(data)-1]
        if lastmeasurement.PacketsSent == lastmeasurement.PacketsRecv {
            desccolor = ""
        } else if lastmeasurement.PacketsRecv == 0 {
            desccolor = "\033[31m"
        } else {
            desccolor = "\033[33m"
        }
    }

    descpart := fmt.Sprintf(fmt.Sprintf("%%-%ds", maxdesc), description)
    if len(descpart) > maxdesc {
        descpart = fmt.Sprintf("%s...", descpart[:maxdesc - 3])
    }

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
            vispart = vispart + "\033[31m█\033[0m"
        } else {
            vispart = vispart + "\033[43m \033[0m"
        }
    }
    // example output: "%s%2s%s"
    formatstring := fmt.Sprintf("%%s%%s\033[0m%%%ds%%s\n", spacing)
    return fmt.Sprintf(formatstring, desccolor, descpart, "", vispart)
}

func updateDisplay(targets []*Target) {
    io.WriteString(os.Stdout, "\033[H\033[2J")
    for index, target := range targets {
        width := getWidth()
        target.Lock()
        line := buildLine(target.Address, target.Data, 18, width)
        target.Unlock()
        io.WriteString(os.Stdout, fmt.Sprintf("\033[%d;0H", index + 1))
        io.WriteString(os.Stdout, line)
    }
}

func uiLoop(uiupdate chan struct{}, targets []*Target) {
    // fill the screen initially
    updateDisplay(targets)

    // wait for an update, then repaint the screen
    for range uiupdate {
        updateDisplay(targets)
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

    // command line arguments
    interval := flag.Float64("i", 1.0, "update interval")
    count := flag.Int("c", 3, "echo requests per interval")
    flag.Parse()

    settings := Settings{}
    settings.Interval = time.Duration(*interval * float64(time.Second))
    settings.PacketInterval = time.Duration(int64(settings.Interval) / int64(2 * *count))
    settings.Timeout = time.Duration(settings.Interval / 2)
    settings.Count = *count

    // FIXME DEBUG
    var targets []*Target
    for _, address := range flag.Args() {
        targets = append(targets, makeTarget(address, &settings))
    }
    if len(targets) == 0 {
        // nothing to do...
        fmt.Printf("Usage of %s: [OPTIONS] target...\n", os.Args[0])
        flag.PrintDefaults()
        return
    }

    for range targets {
        fmt.Println("")
    }
    uiupdate := make(chan struct{})

    // enter the polling main loop
    go updateLoop(targets, uiupdate, &settings)
    go uiLoop(uiupdate, targets)

    //displayList(targets)

    select {
        case <-stopped:
            break
    }
}
