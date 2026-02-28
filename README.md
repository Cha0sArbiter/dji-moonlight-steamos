## DJI Moonlight GUI (SteamOS / Steam Deck)

This is a small Go + Fyne GUI wrapper around the existing `moonlight` CLI
from this repository. It is intended to run on SteamOS / Steam Deck and
make it easy to pair and start a Moonlight stream to DJI FPV goggles
using the `dji_usb` or `dji_net` platforms.

### Prerequisites on Steam Deck

- A working `moonlight` binary built from this repo and placed alongside
  the GUI (for example, both `moonlight` and the `gui` folder in the same
  directory).
- Sunshine (or compatible Gamestream server) running on the Deck,
  typically bound to `127.0.0.1` and exposing apps like `Steam` or
  `Desktop`.
- DJI FPV goggles with WTFOS and `dji-moonlight-shim` installed; the shim
  should be running in either USB BULK or RNDIS mode depending on which
  platform you select in the GUI.
- Go toolchain (1.21 or later) installed on the Deck:

```bash
sudo pacman -S go
```

### Building the GUI

From the `gui` directory:

```bash
cd gui
go mod tidy
go build -o dji-moonlight-gui
```

This will produce a `dji-moonlight-gui` binary in the `gui` directory.

### Running the GUI

Ensure `moonlight` (the dji fork binary) is in the parent directory of
`gui` or in the current working directory, then:

```bash
cd /path/to/dji-moonlight-embedded
./gui/dji-moonlight-gui
```

The GUI provides:

- **Host**: Gamestream host, usually `127.0.0.1` on the Deck.
- **App**: Name of the app exposed by Sunshine (e.g. `Steam`, `Desktop`).
- **Mode**: `USB (dji_usb)` or `RNDIS (dji_net)`.
- **Resolution**: Simple presets (1440x810, 1920x1080, 1280x720).
- **FPS**: 120 or 60.
- **Bitrate**: In Kbps (e.g. `10000`).

Buttons:

- **Pair**: Runs `moonlight pair <host>` and shows output in the log box.
- **Start Stream / Stop Stream**: Starts or stops a stream using
  `moonlight stream` with the selected options, forwarding stdout/stderr
  into the log view.

When using the `dji_usb` platform you may need to run the GUI as root or
set appropriate udev rules so `moonlight` can access the goggles via
libusb.


