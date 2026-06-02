package main

import (
	"flag"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strings"

	md "github.com/JohannesKaufmann/html-to-markdown"
)

var allFields = []string{"window", "front", "tab", "current", "title", "url"}

func die(format string, args ...any) {
	fmt.Fprintf(os.Stderr, "error: "+format+"\n", args...)
	os.Exit(1)
}

func usage() {
	fmt.Fprint(os.Stderr, `Usage: orion <command> [flags] [args]

Commands:
  list         list all windows and tabs
  read         get page content as markdown
  js <code>    execute JavaScript, return result
  open <url>   open URL in a new tab
  nav <url>    navigate a tab to URL

Flags:
  -w <n>      window index, 1-based (read, js, nav; default: front window)
  -t <n>      tab index, 1-based    (read, js, nav; default: current tab)
  -o <fields> comma-separated output fields (list only)
              fields: window, front, tab, current, title, url

URL for open/nav may be piped via stdin.
`)
	os.Exit(1)
}

func runAS(script string) (string, error) {
	cmd := exec.Command("osascript")
	cmd.Stdin = strings.NewReader(script)
	out, err := cmd.CombinedOutput()
	return strings.TrimSpace(string(out)), err
}

func target(w, t int) string {
	win := "front window"
	if w > 0 {
		win = fmt.Sprintf("window id %d", w)
	}
	if t > 0 {
		return fmt.Sprintf("tab %d of %s", t, win)
	}
	return "current tab of " + win
}

// runJS writes code to a temp file and executes it via AppleScript do JavaScript.
// Using a file avoids all AppleScript string quoting issues.
func runJS(code string, w, t int) (string, error) {
	f, err := os.CreateTemp("", "orioncli-*.js")
	if err != nil {
		return "", fmt.Errorf("temp file: %w", err)
	}
	defer os.Remove(f.Name())
	if _, err = f.WriteString(code); err != nil {
		f.Close()
		return "", err
	}
	f.Close()

	script := fmt.Sprintf(`
set jsCode to (read POSIX file "%s")
tell application "Orion"
	do JavaScript jsCode in %s
end tell`, f.Name(), target(w, t))
	return runAS(script)
}

func checkJSErr(out string) {
	if strings.Contains(out, "Allow JavaScript") {
		die("enable 'Allow JavaScript from Apple Events' in Orion's Develop menu")
	}
	if strings.TrimSpace(out) == "missing value" {
		die("tab is suspended (WebKit freezes background tabs); use 'nav <url>' to load it in the front window first")
	}
}

func cmdList(oFlag string) {
	script := `
tell application "Orion"
	set out to {}
	set fw to front window
	repeat with w in windows
		set wId to (id of w) as text
		set ct to current tab of w
		set tIdx to 0
		repeat with t in tabs of w
			set tIdx to tIdx + 1
			if w is fw then
				set isFront to "true"
			else
				set isFront to "false"
			end if
			if t is ct then
				set isCur to "true"
			else
				set isCur to "false"
			end if
			set end of out to (wId & "	" & isFront & "	" & (tIdx as text) & "	" & isCur & "	" & (name of t) & "	" & (URL of t))
		end repeat
	end repeat
	set AppleScript's text item delimiters to linefeed
	return out as text
end tell`

	raw, err := runAS(script)
	if err != nil {
		die("list: %s", raw)
	}

	fieldIdx := map[string]int{}
	for i, f := range allFields {
		fieldIdx[f] = i
	}

	var cols []int
	if oFlag == "" {
		fmt.Println(strings.Join(allFields, "\t"))
		for i := range allFields {
			cols = append(cols, i)
		}
	} else {
		for _, f := range strings.Split(oFlag, ",") {
			f = strings.TrimSpace(f)
			idx, ok := fieldIdx[f]
			if !ok {
				die("unknown field %q (valid: %s)", f, strings.Join(allFields, ","))
			}
			cols = append(cols, idx)
		}
	}

	for _, line := range strings.Split(raw, "\n") {
		if line == "" {
			continue
		}
		fields := strings.SplitN(line, "\t", 6)
		if len(fields) != 6 {
			continue
		}
		row := make([]string, len(cols))
		for i, c := range cols {
			row[i] = fields[c]
		}
		fmt.Println(strings.Join(row, "\t"))
	}
}

// expandAndSerialize expands all relative href/src attributes to absolute using
// the browser's own URL resolution, then returns the full outerHTML.
const expandAndSerialize = `(function(){
	document.querySelectorAll('[href]').forEach(function(el){try{el.setAttribute('href',el.href);}catch(e){}});
	document.querySelectorAll('[src]').forEach(function(el){try{el.setAttribute('src',el.src);}catch(e){}});
	return document.documentElement.outerHTML;
})()`

func cmdRead(w, t int) {
	html, err := runJS(expandAndSerialize, w, t)
	if err != nil {
		checkJSErr(html)
		die("read: %s", html)
	}
	checkJSErr(html)

	converter := md.NewConverter("", true, nil)
	markdown, err := converter.ConvertString(html)
	if err != nil {
		die("read: %v", err)
	}
	fmt.Println(markdown)
}

func cmdJS(code string, w, t int) {
	out, err := runJS(code, w, t)
	if err != nil {
		checkJSErr(out)
		die("js: %s", out)
	}
	checkJSErr(out)
	fmt.Println(out)
}

func urlArg(fs *flag.FlagSet) string {
	if fs.NArg() > 0 {
		return fs.Arg(0)
	}
	stat, _ := os.Stdin.Stat()
	if stat.Mode()&os.ModeCharDevice == 0 {
		data, _ := io.ReadAll(os.Stdin)
		return strings.TrimSpace(string(data))
	}
	return ""
}

func cmdOpen(url string) {
	script := fmt.Sprintf(`
tell application "Orion"
	make new tab at end of tabs of front window with properties {URL:"%s"}
end tell`, url)
	if out, err := runAS(script); err != nil {
		die("open: %s", out)
	}
}

func cmdNav(url string, w, t int) {
	script := fmt.Sprintf(`tell application "Orion" to set URL of %s to "%s"`, target(w, t), url)
	if out, err := runAS(script); err != nil {
		die("nav: %s", out)
	}
}

func main() {
	if len(os.Args) < 2 {
		usage()
	}

	cmd := os.Args[1]
	rest := os.Args[2:]

	fs := flag.NewFlagSet(cmd, flag.ExitOnError)
	wFlag := fs.Int("w", 0, "window index (1-based)")
	tFlag := fs.Int("t", 0, "tab index (1-based)")
	oFlag := fs.String("o", "", "output fields")

	switch cmd {
	case "list":
		fs.Parse(rest)
		cmdList(*oFlag)
	case "read":
		fs.Parse(rest)
		cmdRead(*wFlag, *tFlag)
	case "js":
		fs.Parse(rest)
		if fs.NArg() < 1 {
			die("js requires a code argument")
		}
		cmdJS(fs.Arg(0), *wFlag, *tFlag)
	case "open":
		fs.Parse(rest)
		url := urlArg(fs)
		if url == "" {
			die("open requires a URL")
		}
		cmdOpen(url)
	case "nav":
		fs.Parse(rest)
		url := urlArg(fs)
		if url == "" {
			die("nav requires a URL")
		}
		cmdNav(url, *wFlag, *tFlag)
	default:
		usage()
	}
}
