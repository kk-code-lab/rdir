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
- **c/C (pager)**: Copy visible view/all content to clipboard
- **f (pager)**: Toggle formatted/raw preview (pretty JSON/Markdown when available; falls back to raw for truncated/large files)
- **←/Backspace**: Go to parent
- **/**: Fuzzy search
- **r**: Refresh current directory listing
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
