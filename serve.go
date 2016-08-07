package main

import (
	"flag"
	"fmt"
	"html/template"
	"io/ioutil"
	"log"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strings"
)

var (
	host     string
	port     string
	index    string
	noList   bool
	verbose  bool
	version  = "HEAD"
	htmlTmpl = template.Must(template.New("html").Parse(html))
)

const (
	html = `<!DOCTYPE html>
<html>
<head>
	<meta charset="UTF-8">
	<style>
		body {
			font-size: 14px;
			font-family: consolas, "Liberation Mono", "DejaVu Sans Mono", Menlo, monospace;
		}
		a {
			display: block;
			color: blue;
			text-decoration: none;
		}
		a:hover {
			background-color: #f3f3f3;
		}
		.req-path {
			color: #bbb;
		}
	</style>
</head>
<body>
	{{range .}}
		<h3>
			<span class=local-path>{{.LocalPath}}</span><span class=req-path>{{.RequestPath}}</span>
		</h3>
		{{range .Entries}}
			<a class="entry{{if .IsDir}} dir{{end}}" href={{.Name}}>{{.Name}}{{if .IsDir}}/{{end}}</a>
		{{end}}
	{{end}}
</body>
`
	usage = `
NAME:
   Serve - HTTP server for files spanning multiple directories

USAGE:
   %s [OPTION]... [DIR]...

VERSION:
   %s

OPTIONS:
   -p, --port     --  bind to port (default: 8080)
       --host     --  bind to host (default: localhost)
   -i, --index    --  serve all paths to index if file not found
       --no-list  --  disable file listings
   -v, --verbose  --  display extra information
`
)

func main() {
	flags := getFlags()
	serve(flags)
}

func getFlags() *flag.FlagSet {
	flags := flag.NewFlagSet("flags", flag.ContinueOnError)
	flags.Usage = func() {
		usageName := filepath.Base(os.Args[0])
		fmt.Printf(usage, usageName, version)
	}
	flags.StringVar(&port, "port", "8080", "")
	flags.StringVar(&port, "p", "8080", "")
	flags.StringVar(&host, "host", "localhost", "")
	flags.StringVar(&index, "index", "", "")
	flags.StringVar(&index, "i", "", "")
	flags.BoolVar(&noList, "no-list", false, "")
	flags.BoolVar(&verbose, "verbose", false, "")
	flags.BoolVar(&verbose, "v", false, "")
	err := flags.Parse(os.Args[1:])
	if err == flag.ErrHelp {
		os.Exit(0)
	}
	if err != nil {
		os.Exit(1)
	}
	return flags
}

func serve(flags *flag.FlagSet) {
	dirs := make([]string, flags.NArg())
	for i := range dirs {
		dirs[i] = flags.Arg(i)
	}
	if len(dirs) == 0 {
		dirs = []string{"."}
	}
	http.HandleFunc("/", makeHandler(dirs))
	address := net.JoinHostPort(host, port)
	log.Printf("starting on: http://%s\n", address)
	log.Fatal(http.ListenAndServe(address, nil))
}

func makeHandler(dirs []string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		server := fmt.Sprintf("serve/%s", version)
		w.Header().Set("Server", server)
		if verbose {
			log.Printf("%s → %s %s %s", r.RemoteAddr, r.Method, r.URL.Path, r.Method)
		}
		if !validRequest(r) {
			http.Error(w, "invalid path", http.StatusBadRequest)
			log.Printf("invalid path: %s", r.URL.Path)
			return
		}
		if tryFiles(w, r, dirs) {
			return
		}
		if !strings.Contains(r.Header.Get("Accept"), "text/html") {
			return
		}
		if len(index) > 0 && staticIndex(w, r) {
			return
		}
		if !noList && tryDirs(w, r, dirs) {
			return
		}
		http.NotFound(w, r)
	}
}

func validRequest(r *http.Request) bool {
	if !strings.Contains(r.URL.Path, "..") {
		return true
	}
	for _, field := range strings.FieldsFunc(r.URL.Path, isSlashRune) {
		if field == ".." {
			return false
		}
	}
	return true
}

func isSlashRune(r rune) bool { return r == '/' || r == '\\' }

func tryFiles(w http.ResponseWriter, r *http.Request, dirs []string) bool {
	for _, dir := range dirs {
		filePath := filepath.Join(dir, r.URL.Path)
		indexPath := filepath.Join(filePath, "index.html")
		if tryFile(w, r, filePath) || tryFile(w, r, indexPath) {
			return true
		}
	}
	return false
}

func tryFile(w http.ResponseWriter, r *http.Request, filePath string) bool {
	stat, statErr := os.Stat(filePath)
	if statErr != nil || stat.IsDir() {
		return false
	}
	file, fileErr := os.Open(filePath)
	if fileErr != nil {
		return false
	}
	if verbose {
		log.Printf("%s ← %s", r.RemoteAddr, filePath)
	}
	http.ServeContent(w, r, stat.Name(), stat.ModTime(), file)
	return true
}

func staticIndex(w http.ResponseWriter, r *http.Request) bool {
	file, fileErr := os.Open(index)
	stat, statErr := os.Stat(index)
	if fileErr != nil || statErr != nil {
		log.Println(fileErr)
		return false
	}
	http.ServeContent(w, r, stat.Name(), stat.ModTime(), file)
	return true
}

type dirList struct {
	LocalPath   string
	RequestPath string
	Entries     []entry
}

type entry struct {
	Name  string
	IsDir bool
}

func tryDirs(w http.ResponseWriter, r *http.Request, dirs []string) bool {
	dirLists := []dirList{}
	found := false
	for _, dir := range dirs {
		dirPath := filepath.Join(dir, r.URL.Path)
		dirInfo, err := ioutil.ReadDir(dirPath)
		if err != nil {
			continue
		}
		entries := []entry{
			{
				Name:  "..",
				IsDir: true,
			},
		}
		for _, file := range dirInfo {
			entries = append(entries, entry{
				Name:  file.Name(),
				IsDir: file.IsDir(),
			})
		}
		dirLists = append(dirLists, dirList{
			LocalPath:   dir,
			RequestPath: r.URL.Path,
			Entries:     entries,
		})
		found = true
	}
	if found {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		htmlTmpl.Execute(w, dirLists)
	}
	return found
}
