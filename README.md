# gith

Browse open GitHub issues and pull requests in the terminal. Read the thread, write comments, and open items in your browser.

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

Browse a different repository. For GitHub Enterprise, put the host first:

```bash
gith -R owner/name
gith -R github.example.com/owner/name
```

Load more than 30 open items:

```bash
gith -L 50
```

## Keys

| Key | Action |
| --- | --- |
| `←` `→` | Switch between issues and pull requests |
| `↑` `↓` | Move in the list, or move through the thread of the open item |
| `enter` | Open the selected item |
| `o` | Open the selected item in your browser |
| `r` | Load the lists or the open item again |
| `c` | Write a comment on the open item |
| `ctrl+s` | Send the comment (`ctrl+enter` also works in terminals that support it) |
| `esc` | Close the comment box, or return to the list |
| `q` | Quit |

In an open item, one post fills the screen. The thread holds the description, the comments, and the reviews, oldest first. The footer shows the place in the thread, such as `4/7`. `page up`, `page down`, `home`, and `end` scroll inside that post.

In the comment box, `enter` starts a new line. `esc` closes the box and keeps your text until you send it.
