// Diagnostic: open every PDF in a folder with the library and report any
// failures. Encrypted files are retried with a small list of common
// passwords ("password", "pass"); a file that opens with one of them is
// reported as OK (with the password), otherwise as still-locked (not a
// bug). Panics are caught per-file so one bad input does not abort the run.
//
// By default only the files directly in <folder> are scanned. Pass
// --recurse to walk every subdirectory too.
//
// Usage:  go run ./_examples/openall [--recurse] <folder>
package main

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	pdf "github.com/aspose-pdf-foss/aspose-pdf-foss-for-go"
)

// passwordsToTry are attempted, in order, against encrypted files. The
// empty string comes first to catch the common "encrypted but freely
// readable" pattern (empty user password, non-empty owner password) —
// such files are encrypted, so Open returns ErrEncrypted, yet any viewer
// opens them without a prompt.
var passwordsToTry = []string{"", "password", "pass", "testpassword"}

type result struct {
	path     string
	status   string // "ok", "ok-pw", "locked", "error", "panic"
	pages    int
	password string // which password unlocked it (status "ok-pw")
	detail   string // error / panic message
	elapsed  time.Duration
}

// collectPDFs returns the .pdf files to scan. By default only the files
// directly in dir are returned; when recurse is true every subdirectory
// is walked too. Unreadable entries are skipped rather than aborting.
func collectPDFs(dir string, recurse bool) ([]string, error) {
	var files []string
	if recurse {
		err := filepath.WalkDir(dir, func(path string, d fs.DirEntry, err error) error {
			if err != nil {
				return nil // skip unreadable entries, keep walking
			}
			if !d.IsDir() && strings.EqualFold(filepath.Ext(path), ".pdf") {
				files = append(files, path)
			}
			return nil
		})
		return files, err
	}
	ents, err := os.ReadDir(dir)
	if err != nil {
		return nil, err
	}
	for _, e := range ents {
		if !e.IsDir() && strings.EqualFold(filepath.Ext(e.Name()), ".pdf") {
			files = append(files, filepath.Join(dir, e.Name()))
		}
	}
	return files, nil
}

func main() {
	// Parse args: an optional --recurse flag (in any position) plus the
	// target folder. Kept hand-rolled to avoid a flag-package dependency.
	var dir string
	var recurse bool
	for _, a := range os.Args[1:] {
		switch a {
		case "--recurse", "-recurse":
			recurse = true
		default:
			if dir == "" {
				dir = a
			}
		}
	}
	if dir == "" {
		fmt.Fprintln(os.Stderr, "usage: go run ./_examples/openall [--recurse] <folder>")
		os.Exit(2)
	}

	files, err := collectPDFs(dir, recurse)
	if err != nil {
		fmt.Fprintf(os.Stderr, "scan %q: %v\n", dir, err)
		os.Exit(1)
	}
	sort.Strings(files)

	if len(files) == 0 {
		fmt.Printf("No .pdf files found in %q\n", dir)
		return
	}
	scope := "in"
	if recurse {
		scope = "under"
	}
	fmt.Printf("Opening %d PDF file(s) %s %s\n", len(files), scope, dir)
	fmt.Printf("Passwords tried on encrypted files: %v\n\n", passwordsToTry)

	var results []result
	var nOK, nPW, nLocked, nErr, nPanic int
	for _, f := range files {
		r := openOne(f)
		results = append(results, r)
		switch r.status {
		case "ok":
			nOK++
		case "ok-pw":
			nPW++
		case "locked":
			nLocked++
		case "error":
			nErr++
		case "panic":
			nPanic++
		}
	}

	for _, r := range results {
		name := filepath.Base(r.path)
		switch r.status {
		case "ok":
			fmt.Printf("  OK        %-40s %d page(s)  %v\n", name, r.pages, r.elapsed.Round(time.Millisecond))
		case "ok-pw":
			fmt.Printf("  OK (pw)   %-40s %d page(s)  password=%q\n", name, r.pages, r.password)
		case "locked":
			fmt.Printf("  LOCKED    %-40s (encrypted — none of %v worked)\n", name, passwordsToTry)
		case "error":
			fmt.Printf("  ERROR     %-40s %s\n", name, r.detail)
		case "panic":
			fmt.Printf("  PANIC     %-40s %s\n", name, r.detail)
		}
	}

	fmt.Printf("\nSummary: %d ok, %d ok-with-password, %d locked, %d error, %d panic  (of %d)\n",
		nOK, nPW, nLocked, nErr, nPanic, len(files))

	if nErr > 0 || nPanic > 0 {
		fmt.Println("\nFailures:")
		for _, r := range results {
			if r.status == "error" || r.status == "panic" {
				fmt.Printf("  [%s] %s\n      %s\n", strings.ToUpper(r.status), r.path, r.detail)
			}
		}
		os.Exit(1)
	}
}

// openOne opens a single file, catching panics so the run continues.
// Encrypted files are retried with each candidate password.
func openOne(path string) (r result) {
	r.path = path
	start := time.Now()
	defer func() {
		r.elapsed = time.Since(start)
		if rec := recover(); rec != nil {
			r.status = "panic"
			r.detail = fmt.Sprintf("%v", rec)
		}
	}()

	doc, err := pdf.Open(path)
	if err == nil {
		r.pages = doc.PageCount()
		r.status = "ok"
		return r
	}
	if errors.Is(err, pdf.ErrEncrypted) {
		// Try the candidate passwords (each is tried as both user and
		// owner password by OpenWithPassword).
		for _, pw := range passwordsToTry {
			if doc, perr := openWithPasswordSafe(path, pw); perr == nil {
				r.pages = doc.PageCount()
				r.status = "ok-pw"
				r.password = pw
				return r
			}
		}
		r.status = "locked"
		return r
	}
	r.status = "error"
	r.detail = err.Error()
	return r
}

// openWithPasswordSafe wraps OpenWithPassword, converting a panic into an
// error so one malformed encrypted file can't abort the run.
func openWithPasswordSafe(path, pw string) (doc *pdf.Document, err error) {
	defer func() {
		if rec := recover(); rec != nil {
			doc, err = nil, fmt.Errorf("panic: %v", rec)
		}
	}()
	return pdf.OpenWithPassword(path, pw)
}
