# gith

Browse open GitHub issues and pull requests in the terminal.

`gith` uses the repository in the current directory. It loads data with the [GitHub CLI](https://cli.github.com/).

## Install

Install [Go](https://go.dev/dl/) and the GitHub CLI. Sign in:

```bash
gh auth login
```

Install `gith`:

```bash
go install github.com/jamesofficer/gith/cmd/gith@latest
```

## Use

Open the repository you want to browse, then run:

```bash
gith
```

Browse a different repository:

```bash
gith -R owner/name
```

Load more than 30 open items:

```bash
gith -L 50
```

## Keys

| Key | Action |
| --- | --- |
| `←` `→` | Switch between issues and pull requests |
| `↑` `↓` | Move in the list, or scroll the open item |
| `enter` | Open the selected item |
| `esc` | Return to the list |
| `q` | Quit |

In an open item, `page up`, `page down`, `home`, and `end` also scroll.
