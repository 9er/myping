package main

import (
	"bufio"
	"flag"
	"fmt"
	"github.com/go-ping/ping"
	"io"
	"os"
	"os/signal"
	"strings"
	"sync"
	"syscall"
	"time"
	"unsafe"
)

const VersionString = "v0.1"
const TargetNameLength = 18

type winsize struct {
	Row    uint16
	Col    uint16
	Xpixel uint16
	Ypixel uint16
}

type Settings struct {
	Timeout             time.Duration
	Interval            time.Duration
	PacketInterval      time.Duration
	Count               int
	ScreenWidth         uint16
	TargetNameLength    uint16
	MeasurementCapacity uint16
}

type Measurement struct {
	AvgRTT      time.Duration
	PacketsSent int
	PacketsRecv int
}

type Target struct {
	Address      string
	DisplayName  string
	Measurements []Measurement
}

type DataStore struct {
	sync.Mutex
	Targets []*Target
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
		AvgRTT:      stats.AvgRtt,
		PacketsSent: stats.PacketsSent,
		PacketsRecv: stats.PacketsRecv,
	}

	target.Measurements = append(target.Measurements, measurement)
	// trim the measurement slice to the maximum capacity length
	if uint16(len(target.Measurements)) > settings.MeasurementCapacity {
		target.Measurements = target.Measurements[uint16(len(target.Measurements))-settings.MeasurementCapacity:]
	}
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

func buildLine(description string, target *Target, settings *Settings) string {
	datalen := settings.ScreenWidth - settings.TargetNameLength - 2

	descpart := fmt.Sprintf(fmt.Sprintf("%%-%ds", settings.TargetNameLength), description)
	if uint16(len(descpart)) > settings.TargetNameLength {
		descpart = fmt.Sprintf("%s...", descpart[:settings.TargetNameLength-3])
	}

	var desccolor string

	if len(target.Measurements) == 0 {
		// no measurements, only return the target name
		return descpart
	}

	// the newest measurement defines the color for the target name
	if target.Measurements[len(target.Measurements)-1].PacketsSent == target.Measurements[len(target.Measurements)-1].PacketsRecv {
		desccolor = ""
	} else if target.Measurements[len(target.Measurements)-1].PacketsRecv == 0 {
		desccolor = "\033[31m"
	} else {
		desccolor = "\033[33m"
	}

	var vispart string
	if uint16(len(target.Measurements)) < datalen {
		// fill from the left with empty spaces
		vispart = fmt.Sprintf(fmt.Sprintf("%%%ds", datalen-uint16(len(target.Measurements))), "")
	}

	lastcolor := ""
	charcolor := ""
	resetcolor := "\033[0m"
	char := ""
	// visibleMeasurements is only the most recent part of the series that fits on the screen
	var visibleMeasurements []Measurement
	if uint16(len(target.Measurements)) > datalen {
		visibleMeasurements = target.Measurements[uint16(len(target.Measurements))-datalen:]
	} else {
		visibleMeasurements = target.Measurements
	}
	for _, measurement := range visibleMeasurements {
		// figure out the color and character to print for each measurement
		if measurement.PacketsSent == measurement.PacketsRecv {
			if measurement.PacketsSent == 0 {
				// no echo requests sent -> print a space character (no color)
				char = " "
				charcolor = "\033[0m" // reset to default
			} else {
				// all replies received -> green
				char = "█"
				charcolor = "\033[32m" // green
			}
		} else if measurement.PacketsRecv == 0 {
			// requests sent, but 0 replies received -> red
			char = "█"
			charcolor = "\033[31m" // red
		} else {
			// some (not all and not 0) replies received
			char = "█"
			charcolor = "\033[33m" // yellow
		}

		// append to line
		if charcolor == lastcolor {
			vispart = vispart + char
		} else {
			vispart = vispart + charcolor + char
			lastcolor = charcolor
		}
	}

	// reset color at the end
	vispart = vispart + resetcolor

	// example output: "%s%2s%s"
	formatstring := fmt.Sprintf("%%s%%s\033[0m%%%ds%%s\n", 2)
	return fmt.Sprintf(formatstring, desccolor, descpart, "", vispart)
}

func drawDisplay(store *DataStore, settings *Settings) {
	// print one line for each target
	for index, target := range store.Targets {
		line := buildLine(target.DisplayName, target, settings)
		io.WriteString(os.Stdout, fmt.Sprintf("\033[%d;0H", index+1))
		io.WriteString(os.Stdout, line)
	}
}

func checkScreenSize(store *DataStore, settings *Settings) {
	newwidth := getWidth()
	// check if the screen width changed
	if newwidth != settings.ScreenWidth {
		// set new values, if they are sane
		if newwidth > settings.TargetNameLength+2 {
			// resize the capacity
			// use the biggest seen window width, so a window
			// can be made smaller and wider without losing history
			capacity := newwidth - settings.TargetNameLength - 2
			if capacity > settings.MeasurementCapacity {
				settings.MeasurementCapacity = capacity
			}

			settings.ScreenWidth = newwidth
		}

	}
}

func uiLoop(uiupdate chan struct{}, store *DataStore, settings *Settings) {
	// fill the screen initially
	drawDisplay(store, settings)

	// wait for an update, then repaint the screen
	for range uiupdate {
		store.Lock()
		// reset the cursor and redraw
		io.WriteString(os.Stdout, "\033[u")
		drawDisplay(store, settings)
		store.Unlock()
	}
}

func makeTarget(address string, displayname string, settings *Settings) Target {
	target := Target{
		Address:      address,
		DisplayName:  displayname,
		Measurements: make([]Measurement, settings.MeasurementCapacity),
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
	settings.PacketInterval = time.Duration(int64(settings.Interval) / int64(2**count))
	settings.Timeout = time.Duration(settings.Interval / 2)
	settings.Count = *count
	settings.TargetNameLength = TargetNameLength

	var store DataStore

	// check the screen size, so the data slices can be initialized with correct length
	checkScreenSize(&store, &settings)

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
			// clear the screen
			io.WriteString(os.Stdout, "\033[H\033[2J")
			io.WriteString(os.Stdout, "\033[s")
			// update the screen
			uiupdate <- struct{}{}
			store.Unlock()
		}
	}()

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
