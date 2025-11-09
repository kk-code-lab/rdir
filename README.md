# rdir

Terminal file manager in Go.

## Installation

```bash
go install github.com/kk-code-lab/rdir/cmd/rdir@latest
```

## Usage

```bash
rdir
```

### Keybindings

- **↑/↓**: Navigate files
- **Enter**: Enter directory
- **→**: Open file in pager
- **←/Backspace**: Go to parent
- **/**: Fuzzy search
- **[/]**: History navigation
- **h**: Toggle hidden files
- **q**: Exit

## Building from source

```bash
git clone https://github.com/kk-code-lab/rdir
cd rdir
make build
./build/rdir
```

## Testing

```bash
make test
```

## License

MIT
