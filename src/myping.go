package main

import (
    "github.com/go-ping/ping"
    "fmt"
    "flag"
    "time"
    "sync"
    "io"
    "bufio"
    "strings"
    "os"
    "os/signal"
    "syscall"
    "unsafe"
)

const VersionString = "v0.1"

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
    ScreenWidth uint16
}

type Measurement struct {
    RTT time.Duration
    PacketsSent int
    PacketsRecv int
    Next *Measurement
}

type Target struct {
    Address string
    DisplayName string
    OldestMeasurement *Measurement
    NewestMeasurement *Measurement
    Size uint16
    Capacity uint16
}
func (t *Target) AddMeasurement(newMeasurement *Measurement) {
    if t.Size == uint16(0) {
        t.OldestMeasurement = newMeasurement
        t.NewestMeasurement = newMeasurement
    }

    t.NewestMeasurement.Next = newMeasurement
    t.NewestMeasurement = newMeasurement
    t.Size += 1
    if t.Size > t.Capacity {
        t.OldestMeasurement = t.OldestMeasurement.Next
        t.Size -= 1
    }
}
func (t *Target) SetCapacity(newCapacity uint16) {
    if t.Size > newCapacity {
        current := t.OldestMeasurement
        for i := uint16(0); i < t.Size - newCapacity; i++ {
            current = current.Next
        }
        t.Size = newCapacity
        t.OldestMeasurement = current
    }
    t.Capacity = newCapacity
}

type DataStore struct {
    sync.Mutex
    Targets []*Target
}
func (s *DataStore) SetTargetCapacities(newCapacity uint16) {
    for _, target := range s.Targets {
        target.SetCapacity(newCapacity)
    }
}

const (
    ICMP = iota
    TCP
)

func getWidth() uint16 {
    ws := &winsize{}
    ret, _, err := syscall.Syscall(syscall.SYS_IOCTL, uintptr(syscall.Stdin), uintptr(syscall.TIOCGWINSZ), uintptr(unsafe.Pointer(ws)))

    if int(ret) == -1 {
        panic(err)
    }
    return ws.Col
}

func probeTarget(wg *sync.WaitGroup, target *Target, settings *Settings) {
    defer wg.Done()

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

    target.AddMeasurement(&measurement)
}

func updateLoop(store *DataStore, uiupdate chan struct{}, settings *Settings) {
    // periodically start new measurements
    update(store, uiupdate, settings)
    ticker := time.NewTicker(settings.Interval)
    for range ticker.C {
        update(store, uiupdate, settings)
	}
}

func update(store *DataStore, uiupdate chan struct{}, settings *Settings) {
    store.Lock()
    wg := sync.WaitGroup{}
    for _, target := range store.Targets {
        wg.Add(1)
        go probeTarget(&wg, target, settings)
    }
    wg.Wait()
    store.Unlock()
    uiupdate <- struct{}{}
}

func buildLine(description string, target *Target, maxdesc uint16, maxwidth uint16) string {
    spacing := uint16(2)
    datalen := maxwidth - maxdesc - spacing

    var desccolor string
    descpart := fmt.Sprintf(fmt.Sprintf("%%-%ds", maxdesc), description)
    if uint16(len(descpart)) > maxdesc {
        descpart = fmt.Sprintf("%s...", descpart[:maxdesc - 3])
    }

    if target.Size > 0 {
        if target.NewestMeasurement.PacketsSent == target.NewestMeasurement.PacketsRecv {
            desccolor = ""
        } else if target.NewestMeasurement.PacketsRecv == 0 {
            desccolor = "\033[31m"
        } else {
            desccolor = "\033[33m"
        }
    } else {
        // no measurements
        return descpart
    }

    var vispart string
    if target.Size < datalen {
        // fill from the left with empty spaces
        vispart = fmt.Sprintf(fmt.Sprintf("%%%ds", datalen - target.Size), "")
    }

    lastcolor := ""
    charcolor := ""
    resetcolor := "\033[0m"
    char := ""
    // figure out the color and character to print for each measurement
    currentMeasurement := target.OldestMeasurement
    for index := uint16(0); index < target.Size; index++ {
        if currentMeasurement.PacketsSent == currentMeasurement.PacketsRecv {
            if currentMeasurement.PacketsSent == 0 {
                char = " "
                charcolor = "\033[0m"  // reset to default
            } else {
                char = "█"
                charcolor = "\033[32m"  // green
            }
        } else if currentMeasurement.PacketsRecv == 0 {
            char = "█"
            charcolor = "\033[31m"  // red
        } else {
            char = "█"
            charcolor = "\033[33m"  // yellow
        }

        // append to line
        if charcolor == lastcolor {
            vispart = vispart + char
        } else {
            vispart = vispart + charcolor + char
            lastcolor = charcolor
        }

        currentMeasurement = currentMeasurement.Next
    }

    // reset color at the end
    vispart = vispart + resetcolor

    // example output: "%s%2s%s"
    formatstring := fmt.Sprintf("%%s%%s\033[0m%%%ds%%s\n", spacing)
    return fmt.Sprintf(formatstring, desccolor, descpart, "", vispart)
}

func drawDisplay(store *DataStore, width uint16) {
    // print one line for each target
    for index, target := range store.Targets {
        line := buildLine(target.DisplayName, target, uint16(18), width)
        io.WriteString(os.Stdout, fmt.Sprintf("\033[%d;0H", index + 1))
        io.WriteString(os.Stdout, line)
    }
}

func checkScreenSize(store *DataStore, settings *Settings) {
    newwidth := getWidth()
    // check if the screen width changed
    if newwidth != settings.ScreenWidth {
        // resize the data
        store.SetTargetCapacities(newwidth - 20) // FIXME Uint

        // clear the screen
        io.WriteString(os.Stdout, "\033[H\033[2J")
        io.WriteString(os.Stdout, "\033[s")
        settings.ScreenWidth = newwidth
    }
}

func uiLoop(uiupdate chan struct{}, store *DataStore, settings *Settings) {
    // fill the screen initially
    drawDisplay(store, settings.ScreenWidth)

    // wait for an update, then repaint the screen
    for range uiupdate {
        store.Lock()
        // reset the cursor and redraw
        io.WriteString(os.Stdout, "\033[u")
        drawDisplay(store, settings.ScreenWidth)
        store.Unlock()
    }
}

func makeTarget(address string, displayname string, settings *Settings) Target {
    target := Target{
        Address: address,
        DisplayName: displayname,
        Capacity: uint16(10),
    }
    return target
}

func main() {
    // used to terminate gracefully
    stopped := make(chan os.Signal)
    signal.Notify(stopped, os.Interrupt)

    // command line arguments
    interval := flag.Float64("i", 1.0, "update interval")
    count := flag.Int("c", 3, "echo requests per interval")
    targetfile := flag.String("f", "", "target list file (format: address displayname)")
    // flag package doesn't support "--version" <.<
    version := flag.Bool("v", false, "prints the myping version")
    flag.Parse()

    if *version {
        fmt.Println(VersionString)
        return
    }

    settings := Settings{}
    settings.Interval = time.Duration(*interval * float64(time.Second))
    settings.PacketInterval = time.Duration(int64(settings.Interval) / int64(2 * *count))
    settings.Timeout = time.Duration(settings.Interval / 2)
    settings.Count = *count

    var store DataStore

    if *targetfile == "" {
        // no target file specified → read targets from cmdline arguments
        store.Targets = make([]*Target, len(flag.Args()))
        for index, address := range flag.Args() {
            target := makeTarget(address, address, &settings)
            store.Targets[index] = &target
        }
    } else {
        // read targets from file
        file, err := os.Open(*targetfile)
        if err != nil {
            fmt.Println("Unable to read target list file.")
            fmt.Printf("Usage of %s: [OPTIONS] target...\n", os.Args[0])
            flag.PrintDefaults()
            return
        }
        reader := bufio.NewReader(file)
        var lines []string
        for {
            line, _, err := reader.ReadLine()
            if err == io.EOF {
                 break
            }
            lines = append(lines, string(line))
        }
        store.Targets = make([]*Target, len(lines))
        for index, line := range lines {
            elements := strings.SplitN(line, " ", 2)
            if len(elements) != 2 {
                fmt.Printf("Error in target list: line should contain, target and displayname, separated with space:\n%s\n", line)
                return
            }
            target := makeTarget(elements[0], elements[1], &settings)
            store.Targets[index] = &target
        }
    }

    if len(store.Targets) == 0 {
        // nothing to do...
        fmt.Printf("Usage of %s: [OPTIONS] target...\n", os.Args[0])
        flag.PrintDefaults()
        return
    }

    uiupdate := make(chan struct{})

    // catch SIGWINCH (terminal resize), get the new terminal size and trigger a UI update
    sigwinch := make(chan os.Signal)
    signal.Notify(sigwinch, syscall.SIGWINCH)
    go func() {
        for {
            <-sigwinch
            store.Lock()
            checkScreenSize(&store, &settings)
            uiupdate <- struct{}{}
            store.Unlock()
        }
    }()

    checkScreenSize(&store, &settings)

    // clear the screen and save cursor position
    io.WriteString(os.Stdout, "\033[H\033[2J")
    io.WriteString(os.Stdout, "\033[s")

    // enter the polling main loop
    go updateLoop(&store, uiupdate, &settings)
    go uiLoop(uiupdate, &store, &settings)

    select {
        case <-stopped:
            io.WriteString(os.Stdout, "\033[H\033[2J")
            return
    }
}
