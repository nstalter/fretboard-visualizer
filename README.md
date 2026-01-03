# Fretboard Visualizer

A guitar chord visualization tool built in Go. This application generates and displays guitar chord voicings on an interactive fretboard.

The app attempts to generate playable chord positions but not all generated chords will necessarily be playable in practice.

## Features

- Generate chord voicings for different qualities and keys
- Interactive fretboard display with note positions
- Navigate through different chord positions
- Visual indicators for root notes and barre chords
- Dropdown selection for chord root and type

## Installation

### Prerequisites
- Go 1.19 or later

### Build and Run
```bash
git clone https://github.com/nstalter/fretboard-visualizer.git
cd fretboard-visualizer
go build .
./fretboard-visualizer
```

## Usage

1. Select a chord root from the "Key" dropdown
2. Select a chord type from the "Quality" dropdown
3. View the chord voicing on the fretboard
4. Use navigation buttons to see alternative positions

## License

MIT License
