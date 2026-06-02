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

var browser = "Orion"

func die(format string, args ...any) {
	fmt.Fprintf(os.Stderr, "error: "+format+"\n", args...)
	os.Exit(1)
}

func usage() {
	fmt.Fprint(os.Stderr, `Usage: sxt <command> [flags] [args]

Commands:
  list         list all windows and tabs
  read         get page content as markdown
  js <code>    execute JavaScript, return result
  open <url>   open URL in a new tab
  nav <url>    navigate a tab to URL
  close        close a tab

Flags:
  -b <name>   sxt app name (default: Orion; env: BROWSER_APP)
  -w <id>     window ID from list   (read, js, nav; default: front window)
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
	f, err := os.CreateTemp("", "sxt-*.js")
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
tell application "%s"
	do JavaScript jsCode in %s
end tell`, f.Name(), browser, target(w, t))
	return runAS(script)
}

func checkJSErr(out string) {
	if strings.Contains(out, "Allow JavaScript") {
		die("enable 'Allow JavaScript from Apple Events' in %s's Develop menu", browser)
	}
	if strings.TrimSpace(out) == "missing value" {
		die("tab is suspended (WebKit freezes background tabs); use 'nav <url>' to load it in the front window first")
	}
}

func cmdList(oFlag string) {
	script := fmt.Sprintf(`
tell application "%s"
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
end tell`, browser)

	raw, err := runAS(script)
	if err != nil {
		die("list: %s", raw)
	}

	var cols []int
	if oFlag == "" {
		fmt.Println(strings.Join(allFields, "\t"))
	} else {
		fieldIdx := map[string]int{}
		for i, f := range allFields {
			fieldIdx[f] = i
		}
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
		if cols == nil {
			fmt.Println(line)
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
	checkJSErr(html)
	if err != nil {
		die("read: %s", html)
	}

	converter := md.NewConverter("", true, nil)
	markdown, err := converter.ConvertString(html)
	if err != nil {
		die("read: %v", err)
	}
	fmt.Println(markdown)
}

func cmdJS(code string, w, t int) {
	out, err := runJS(code, w, t)
	checkJSErr(out)
	if err != nil {
		die("js: %s", out)
	}
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
tell application "%s"
	make new tab at end of tabs of front window with properties {URL:"%s"}
end tell`, browser, url)
	if out, err := runAS(script); err != nil {
		die("open: %s", out)
	}
}

func cmdNav(url string, w, t int) {
	script := fmt.Sprintf(`tell application "%s" to set URL of %s to "%s"`, browser, target(w, t), url)
	if out, err := runAS(script); err != nil {
		die("nav: %s", out)
	}
}

func cmdClose(w, t int) {
	script := fmt.Sprintf(`tell application "%s" to close %s`, browser, target(w, t))
	if out, err := runAS(script); err != nil {
		die("close: %s", out)
	}
}

func main() {
	if len(os.Args) < 2 {
		usage()
	}

	if b := os.Getenv("BROWSER_APP"); b != "" {
		browser = b
	}

	cmd := os.Args[1]
	rest := os.Args[2:]

	fs := flag.NewFlagSet(cmd, flag.ExitOnError)
	bFlag := fs.String("b", "", "sxt app name (overrides BROWSER_APP env)")
	wFlag := fs.Int("w", 0, "window ID from list")
	tFlag := fs.Int("t", 0, "tab index (1-based)")
	oFlag := fs.String("o", "", "output fields")

	fs.Parse(rest)
	if *bFlag != "" {
		browser = *bFlag
	}

	switch cmd {
	case "list":
		cmdList(*oFlag)
	case "read":
		cmdRead(*wFlag, *tFlag)
	case "js":
		if fs.NArg() < 1 {
			die("js requires a code argument")
		}
		cmdJS(fs.Arg(0), *wFlag, *tFlag)
	case "open":
		url := urlArg(fs)
		if url == "" {
			die("open requires a URL")
		}
		cmdOpen(url)
	case "nav":
		url := urlArg(fs)
		if url == "" {
			die("nav requires a URL")
		}
		cmdNav(url, *wFlag, *tFlag)
	case "close":
		cmdClose(*wFlag, *tFlag)
	default:
		usage()
	}
}
