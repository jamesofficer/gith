# gith

Browse open GitHub issues and pull requests in the terminal. See who is assigned to each one. Read the thread, write comments, and open items in your browser.

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
| `←` `→` | Switch between issues and pull requests in the list |
| `↑` `↓` | Move in the list, or move through the thread of the open item |
| `enter` | Open the selected item |
| `b` | Open the selected item in your browser |
| `r` | Load the lists or the open item again |
| `c` | Write a comment on the open item |
| `d` | Show the diff of the open pull request |
| `ctrl+s` | Send the comment (`ctrl+enter` also works in terminals that support it) |
| `esc` | Close the comment box, or return to the list |
| `q` | Quit |

In an open item, one post fills the screen. The thread holds the description, the comments, the reviews, and the comments on lines of code, oldest first. A comment on code shows its file, its line, and the diff above it. Its replies go under it with an indent. The top of the post frame shows the place in the thread, such as `4 of 7`. `page up`, `page down`, `home`, and `end` scroll inside that post.

In the diff, one changed file fills the screen, and long lines wrap. `←` and `→` move between files. A cursor marks one line: `↑` `↓` move it 6 lines, and `k` `j` move it 1 line. The left column shows the line number: the new number, or the old number for a removed line. 💬 marks a line that has a comment, and the start of each comment shows under the line. Press `enter` on the line to show its comments in full with their replies, and again to show only the start. `esc` or `d` returns to the thread. A very large pull request can be too large for GitHub to send. Then press `b` to see it in your browser.

To comment on code, press `c` on the first line, move to the last line, and press `c` again. Press `c` two times to comment on one line. A range must stay in one block of changes (`@@`). `esc` cancels the range. The comment is sent at once, not as part of a review.

In the comment box, `enter` and `shift+enter` start a new line. `esc` closes the box and keeps your text until you send it.
