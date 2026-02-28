package main

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/app"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/widget"
)

type streamMode string

const (
	modeUSB  streamMode = "USB (dji_usb)"
	modeRNDIS streamMode = "RNDIS (dji_net)"
)

type guiState struct {
	host      *widget.Entry
	appName   *widget.Entry
	mode      *widget.Select
	res       *widget.Select
	fps       *widget.Select
	bitrate   *widget.Entry
	logOutput *widget.Entry

	pairButton      *widget.Button
	streamButton    *widget.Button
	sunshineButton  *widget.Button

	mu        sync.Mutex
	streamCmd *exec.Cmd
	cancelFn  context.CancelFunc
}

func main() {
	// Ensure a reasonable default scale for small, high-DPI screens like Steam Deck.
	// This will be overridden if FYNE_SCALE is already set in the environment.
	if os.Getenv("FYNE_SCALE") == "" {
		// Smaller than default (1.0) so the UI doesn't appear oversized.
		os.Setenv("FYNE_SCALE", "0.7")
	}

	a := app.NewWithID("dji-moonlight-gui")
	w := a.NewWindow("DJI Moonlight - Steam Deck")
	// Smaller default size so it fits better on Steam Deck and small displays.
	// Fyne will still scale for DPI, but this keeps the logical size compact.
	w.Resize(fyne.NewSize(420, 260))

	state := &guiState{}

	// Inputs
	state.host = widget.NewEntry()
	state.host.SetPlaceHolder("127.0.0.1")
	state.host.SetText("127.0.0.1")

	state.appName = widget.NewEntry()
	state.appName.SetPlaceHolder("Steam or Desktop")
	state.appName.SetText("Steam")

	modes := []string{string(modeUSB), string(modeRNDIS)}
	state.mode = widget.NewSelect(modes, nil)
	state.mode.SetSelected(string(modeUSB))

	resOptions := []string{"1440x810", "1920x1080", "1280x720"}
	state.res = widget.NewSelect(resOptions, nil)
	state.res.SetSelected("1440x810")

	fpsOptions := []string{"120", "60"}
	state.fps = widget.NewSelect(fpsOptions, nil)
	state.fps.SetSelected("120")

	state.bitrate = widget.NewEntry()
	state.bitrate.SetPlaceHolder("Bitrate (Kbps)")
	state.bitrate.SetText("10000")

	// Log output
	state.logOutput = widget.NewMultiLineEntry()
	state.logOutput.SetPlaceHolder("Logs from moonlight will appear here...")
	state.logOutput.Wrapping = fyne.TextWrapWord
	// Keep minimum height small so the window can shrink; content will scroll
	state.logOutput.SetMinRowsVisible(3)
	state.logOutput.Disable()

	// Buttons
	state.pairButton = widget.NewButton("Pair", func() {
		go state.runPair()
	})

	state.streamButton = widget.NewButton("Start Stream", func() {
		go state.toggleStream()
	})

	state.sunshineButton = widget.NewButton("Open Sunshine UI", func() {
		go state.openSunshine()
	})

	mainForm := container.NewGridWithColumns(2,
		widget.NewLabel("Host"),
		state.host,
		widget.NewLabel("App"),
		state.appName,
		widget.NewLabel("Mode"),
		state.mode,
	)

	logScroll := container.NewVScroll(state.logOutput)

	advancedForm := container.NewGridWithColumns(2,
		widget.NewLabel("Resolution"),
		state.res,
		widget.NewLabel("FPS"),
		state.fps,
		widget.NewLabel("Bitrate (Kbps)"),
		state.bitrate,
	)

	mainButtons := container.NewHBox(state.pairButton, state.streamButton, state.sunshineButton)
	mainTabContent := container.NewVBox(mainForm, mainButtons)

	tabs := container.NewAppTabs(
		container.NewTabItem("Main", mainTabContent),
		container.NewTabItem("Advanced", advancedForm),
		container.NewTabItem("Log", logScroll),
	)

	w.SetContent(tabs)

	w.SetCloseIntercept(func() {
		// Stop any running stream before exit
		go func() {
			state.stopStream()
			time.Sleep(200 * time.Millisecond)
			a.Quit()
		}()
	})

	w.ShowAndRun()
}

func (s *guiState) appendLog(line string) {
	if line == "" {
		return
	}
	timePrefix := time.Now().Format("15:04:05")
	full := fmt.Sprintf("[%s] %s\n", timePrefix, line)

	// Only send desktop notifications for important events:
	// - Pairing PIN
	// - Obvious failures/errors
	notify := false
	title := "Moonlight"

	if strings.Contains(line, "Please enter the following PIN") {
		notify = true
		title = "Moonlight Pairing"
	} else if strings.Contains(line, "Failed") ||
		strings.Contains(line, "failed") ||
		strings.Contains(line, "Error") ||
		strings.Contains(line, "error") ||
		strings.Contains(line, "Can't") {
		notify = true
		title = "Moonlight Error"
	}

	if notify {
		fyne.CurrentApp().SendNotification(&fyne.Notification{
			Title:   title,
			Content: line,
		})
	}

	s.logOutput.SetText(s.logOutput.Text + full)
	s.logOutput.CursorRow = len(bytes.Split([]byte(s.logOutput.Text), []byte("\n")))
}

func (s *guiState) moonlightPath() string {
	// Assume GUI is placed next to the moonlight binary or in project root with ./moonlight
	cwd, err := os.Getwd()
	if err != nil {
		return "./moonlight"
	}
	// Prefer ./moonlight in current directory
	p := filepath.Join(cwd, "moonlight")
	return p
}

func (s *guiState) runPair() {
	host := s.host.Text
	if host == "" {
		host = "127.0.0.1"
	}

	s.appendLog("Running: moonlight pair " + host)

	cmd := exec.Command(s.moonlightPath(), "pair", host)
	s.runCommand(cmd)
}

func (s *guiState) openSunshine() {
	host := s.host.Text
	if host == "" {
		host = "127.0.0.1"
	}

	// Sunshine web UI default port is 47990
	url := fmt.Sprintf("http://%s:47990", host)
	s.appendLog("Opening Sunshine UI at " + url)

	var cmd *exec.Cmd
	// On Linux / SteamOS, xdg-open is the standard way to open URLs
	cmd = exec.Command("xdg-open", url)
	_ = cmd.Start()
}

func (s *guiState) toggleStream() {
	s.mu.Lock()
	running := s.streamCmd != nil
	s.mu.Unlock()

	if running {
		s.stopStream()
		return
	}

	s.startStream()
}

func (s *guiState) startStream() {
	host := s.host.Text
	if host == "" {
		host = "127.0.0.1"
	}

	appName := s.appName.Text
	if appName == "" {
		appName = "Steam"
	}

	mode := s.mode.Selected
	if mode == "" {
		mode = string(modeUSB)
	}

	res := s.res.Selected
	if res == "" {
		res = "1440x810"
	}
	var width, height int
	fmt.Sscanf(res, "%dx%d", &width, &height)

	fps := s.fps.Selected
	if fps == "" {
		fps = "120"
	}

	bitrate := s.bitrate.Text
	if bitrate == "" {
		bitrate = "10000"
	}

	var platform string
	switch streamMode(mode) {
	case modeRNDIS:
		platform = "dji_net"
	default:
		platform = "dji_usb"
	}

	args := []string{
		"stream",
		"-platform", platform,
		"-app", appName,
		"-width", fmt.Sprintf("%d", width),
		"-height", fmt.Sprintf("%d", height),
		"-fps", fps,
		"-bitrate", bitrate,
		"-debug",
		host,
	}

	s.appendLog(fmt.Sprintf("Running: moonlight %v", args))

	ctx, cancel := context.WithCancel(context.Background())
	cmd := exec.CommandContext(ctx, s.moonlightPath(), args...)

	s.mu.Lock()
	s.streamCmd = cmd
	s.cancelFn = cancel
	s.mu.Unlock()

	s.setStreamButtonLabel("Stop Stream")

	go func() {
		s.runCommand(cmd)
		s.mu.Lock()
		s.streamCmd = nil
		s.cancelFn = nil
		s.mu.Unlock()
		s.setStreamButtonLabel("Start Stream")
	}()
}

func (s *guiState) stopStream() {
	s.mu.Lock()
	cancel := s.cancelFn
	cmd := s.streamCmd
	s.mu.Unlock()

	if cancel != nil {
		s.appendLog("Stopping stream...")
		cancel()
	}

	if cmd != nil && cmd.Process != nil {
		_ = cmd.Process.Kill()
	}
}

func (s *guiState) setStreamButtonLabel(label string) {
	s.streamButton.SetText(label)
}

func (s *guiState) runCommand(cmd *exec.Cmd) {
	stdout, _ := cmd.StdoutPipe()
	stderr, _ := cmd.StderrPipe()

	if err := cmd.Start(); err != nil {
		s.appendLog(fmt.Sprintf("Failed to start: %v", err))
		return
	}

	var wg sync.WaitGroup
	wg.Add(2)

	readPipe := func(r io.Reader, prefix string) {
		defer wg.Done()
		buf := make([]byte, 4096)
		for {
			n, err := r.Read(buf)
			if n > 0 {
				for _, line := range bytes.Split(buf[:n], []byte("\n")) {
					text := string(bytes.TrimSpace(line))
					if text != "" {
						s.appendLog(fmt.Sprintf("%s%s", prefix, text))
					}
				}
			}
			if err != nil {
				if err != io.EOF {
					s.appendLog(fmt.Sprintf("read error: %v", err))
				}
				return
			}
		}
	}

	go readPipe(stdout, "")
	go readPipe(stderr, "[err] ")

	wg.Wait()
	if err := cmd.Wait(); err != nil {
		s.appendLog(fmt.Sprintf("moonlight exited: %v", err))
	} else {
		s.appendLog("moonlight exited successfully")
	}
}


