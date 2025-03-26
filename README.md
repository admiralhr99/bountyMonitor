# Bounty Monitor

A lightweight service that monitors HackerOne bug bounty programs for changes, helping security researchers stay updated with new opportunities.

## Features

- **Real-time Monitoring**: Tracks new bug bounty programs and scope changes in existing programs
- **Detailed Notifications**: Generates comprehensive reports on changes
- **Flexible Deployment**: Run as a continuous service or in one-off mode
- **Low Resource Consumption**: Efficient design for deployment on any server or VPS

## Installation

### Prerequisites

- Go 1.23 or higher

### Option 1: Install with `go install`

```bash
# Install directly using go install
go install github.com/admiralhr99/bountyMonitor@latest
```

The binary will be installed to your `$GOPATH/bin` directory, which should be in your PATH.

### Option 2: Building from Source

```bash
# Clone the repository
git clone https://github.com/admiralhr99/bountyMonitor.git
cd bounty-monitor

# Build the binary
go build -o bounty-monitor
```

## Usage

### Run as a Service

```bash
# Start the monitoring service (runs continuously)
./bounty-monitor
```

The service will:
- Run an initial check immediately
- Continue checking for updates every hour
- Log all activities to `bounty-monitor.log`
- Store notifications in `.bounty-monitor/notifications.txt`

### One-time Check

```bash
# Run a single check and exit
./bounty-monitor -now
```

## Configuration

Configuration is handled via constants in the code:

| Constant | Description | Default |
|----------|-------------|---------|
| `checkInterval` | Time between checks | 1 hour |
| `cacheDir` | Directory for cached data | `.bounty-monitor` |
| `cacheFile` | Filename for caching previous data | `hackerone_previous.json` |
| `notificationFile` | Filename for notifications | `notifications.txt` |

To modify these settings, edit the constants in the source code and rebuild.

## Data Sources

This tool uses data from the [bounty-targets-data](https://github.com/arkadiyt/bounty-targets-data) repository, which aggregates bug bounty platform information from HackerOne and other platforms.


## Contributing

Contributions are welcome! Please feel free to submit a Pull Request.

## License

This project is licensed under the MIT License - see the LICENSE file for details.

## Acknowledgments

- Data provided by [bounty-targets-data](https://github.com/arkadiyt/bounty-targets-data)