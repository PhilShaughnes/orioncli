# sxt

A minimal CLI for controlling macOS browsers from the shell or an agent.

Supports **Orion** (default) and **Safari**. Any browser with a compatible AppleScript dictionary should work.

---

## Prerequisites

Enable **Develop → Allow JavaScript from Apple Events** in your browser.

Without this, `read` and `js` will error with instructions.

---

## Example

```sh
# What's open?
sxt list

# Read the current tab as markdown
sxt read

# Read a specific tab
sxt list                          # find window ID and tab index
sxt read -w 29178 -t 3

# Run JS in a tab
sxt js "document.title"
sxt js "JSON.stringify([...document.querySelectorAll('h2')].map(h=>h.innerText))"

# Navigate and read
sxt nav https://example.com
sxt read

# Open a new tab
sxt open https://example.com

# Close the current tab
sxt close

# Use Safari instead of Orion
sxt -b Safari list
```

---

## Commands

```
sxt list              list all windows and tabs (tab-separated)
sxt read              get page content as markdown
sxt js <code>         execute JavaScript, return result
sxt nav <url>         navigate a tab to URL
sxt open <url>        open URL in a new tab
sxt close             close a tab
```

**Flags (read, js, nav, close):**

```
-w <id>    window ID from list (default: front window)
-t <n>     tab index, 1-based (default: current tab)
```

**Flags (list):**

```
-o <fields>   comma-separated output fields, no header (pipeline mode)
              fields: window, front, tab, current, title, url
```

**Browser selection:**

```
-b <name>     sxt app name (default: Orion; env: BROWSER_APP)
```

URL for `nav` and `open` may be piped via stdin.

---

## Browser selection

```sh
# one-off
sxt -b Safari list

# per-session default
export BROWSER_APP=Safari
sxt list
```

---

## list output

```
window  front  tab  current  title                   url
29178   true   1    false    Example Domain           https://example.com/
29178   true   2    true     GitHub                   https://github.com/
29224   false  1    false    Hacker News              https://news.ycombinator.com/
```

`window` is a stable ID — it doesn't change as windows move between front and back. Use it with `-w` for reliable targeting across commands.

`front` and `current` indicate the active window and active tab within each window.

---

## Composability

```sh
# All URLs open right now
sxt list -o url

# Titles and URLs for one window
sxt list -o tab,title,url | grep "^29178"

# Open every GitHub URL in a new tab
sxt list -o url | grep github.com | while read -r url; do sxt open "$url"; done

# Pick a tab to read with fzf
sxt list -o window,tab,title \
  | fzf \
  | awk '{print "-w", $1, "-t", $2}' \
  | xargs sxt read

# Send current page to an API
sxt read | curl -s -X POST https://api.example.com/summarize -d @-

# Extract all links from a page
sxt js "JSON.stringify([...document.querySelectorAll('a')].map(a=>a.href))"

# Navigate from stdin
echo "https://example.com" | sxt nav
```

---

## Notes

**`read` and `js` require a visible tab.** WebKit suspends JavaScript in background windows to save memory. If you see:

```
error: tab is suspended (WebKit freezes background tabs); use 'nav <url>' to load it in the front window first
```

Load the page in the front window first:

```sh
sxt nav https://example.com      # loads in front window, current tab
sxt read                         # reads it
```

**`read` returns rendered content.** It gets the post-JavaScript DOM, not raw source — so SPAs and dynamically loaded content are included. Use `sxt js "document.documentElement.outerHTML"` if you need the raw HTML.

---

## Name

A sextant is a navigation instrument. Sailors used it to fix their position by the stars, Orion among them. It turns out *safari* is Swahili for journey. Seemed like the right tool for the trip.

---

## Install

```sh
go install github.com/PhilShaughnes/sxt@latest
```

Or build from source:

```sh
git clone https://github.com/PhilShaughnes/sxt
cd sxt
go build -o sxt .
```
