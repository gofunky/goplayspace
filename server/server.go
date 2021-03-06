package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"flag"
	"io"
	"io/ioutil"
	"log"
	"net/http"
	"net/url"
	"os"
	"strconv"
	"strings"
	"time"
)

const staticDir = "../static"

const userAgent = "goplay.space/1.0 (https://goplay.space/)"

// maxSnippetSize value taken from
// https://github.com/golang/playground/blob/master/app/goplay/share.go
const maxSnippetSize = 64 * 1024

// FmtResponse is the response returned from
// upstream play.golang.org/fmt request
type FmtResponse struct {
	Body  string
	Error string
}

// CompileEvent represents individual
// event record in CompileResponse
type CompileEvent struct {
	Message string
	Kind    string
	Delay   time.Duration
}

// CompileResponse is the response returned from
// upstream play.golang.org/compile request
type CompileResponse struct {
	Body   *string
	Events []*CompileEvent
	Errors string
}

type serverContext struct {
	backend string
}

func gzPath(path string) string {
	return staticDir + path + ".gz"
}

func main() {
	port := flag.Int("p", 8080, "port to listen at")
	help := flag.Bool("h", false, "show this help")
	server := flag.String("s", "https://play.golang.org", "custom playground server adress")

	flag.Parse()

	if *help {
		flag.Usage()
		return
	}

	c := serverContext{*server}

	log.Printf("Listening on http://localhost:%d/", *port)

	http.Handle("/", http.FileServer(http.Dir(staticDir)))
	http.HandleFunc("/compile", c.compileHandler)
	http.HandleFunc("/share", c.shareHandler)
	http.HandleFunc("/load", c.loadHandler)

	if _, err := os.Stat(gzPath("/client.js")); err == nil {
		http.HandleFunc("/client.js", c.gzHandler)
	}
	if _, err := os.Stat(gzPath("/client.js.map")); err == nil {
		http.HandleFunc("/client.js.map", c.gzHandler)
	}

	log.Fatal(http.ListenAndServe(":"+strconv.Itoa(*port), nil))
}

func (c *serverContext) doRequest(method, url, contentType string, body io.Reader) ([]byte, error) {
	req, err := http.NewRequest(method, url, body)
	if err != nil {
		return nil, err
	}
	req.Header.Add("Content-Type", contentType)
	req.Header.Add("User-Agent", userAgent)
	response, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, err
	}

	var bodyBytes bytes.Buffer
	_, err = io.Copy(&bodyBytes, io.LimitReader(response.Body, maxSnippetSize+1))
	response.Body.Close()
	if err != nil {
		return nil, err
	}
	if bodyBytes.Len() > maxSnippetSize {
		return nil, errors.New("Snippet is too large")
	}
	return bodyBytes.Bytes(), nil
}

func (c *serverContext) postForm(url string, data url.Values) ([]byte, error) {
	return c.doRequest("POST", url, "application/x-www-form-urlencoded", strings.NewReader(data.Encode()))
}

func (c *serverContext) runShare(body io.Reader) ([]byte, error) {
	return c.doRequest("POST", strings.Join([]string{c.backend, "share"}, "/"), "text/plain", body)
}

func (c *serverContext) runImports(body *string) ([]byte, error) {
	form := url.Values{}
	form.Add("imports", "true")
	form.Add("body", *body)

	return c.postForm(strings.Join([]string{c.backend, "fmt"}, "/"), form)
}

func (c *serverContext) runCompile(body *string) ([]byte, error) {
	form := url.Values{}
	form.Add("body", *body)
	form.Add("version", "2")

	return c.postForm(strings.Join([]string{c.backend, "compile"}, "/"), form)
}

func (c *serverContext) compileHandler(w http.ResponseWriter, r *http.Request) {
	defer r.Body.Close()

	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	bodyBytes, err := ioutil.ReadAll(r.Body)
	if err != nil {
		http.Error(w, "Failed to read request data", http.StatusInternalServerError)
		return
	}

	body := string(bodyBytes)

	bodyBytes, err = c.runImports(&body)
	if err != nil {
		log.Printf("runImports() error: %v", err)
		http.Error(w, "Failed to format source code", http.StatusInternalServerError)
		return
	}

	fmtResponse := FmtResponse{}

	err = json.Unmarshal(bodyBytes, &fmtResponse)
	if err != nil {
		log.Printf("fmtResponse unmarshal error: %v", err)
		http.Error(w, "Failed to decode upstream server response", http.StatusInternalServerError)
		return
	}

	if fmtResponse.Error != "" {
		w.Write(bodyBytes)
		return
	}

	bodyUpdated := fmtResponse.Body != body

	bodyBytes, err = c.runCompile(&fmtResponse.Body)
	if err != nil {
		log.Printf("runCompile() error: %v", err)
		http.Error(w, "Failed to compile source code", http.StatusInternalServerError)
		return
	}

	if !bodyUpdated {
		w.Write(bodyBytes)
		return
	}

	// return a new payload with the formatted body

	compileResponse := CompileResponse{}

	err = json.Unmarshal(bodyBytes, &compileResponse)
	if err != nil {
		log.Printf("compileResponse unmarshal error: %v", err)
		http.Error(w, "Failed to decode upstream server response", http.StatusInternalServerError)
		return
	}

	compileResponse.Body = &fmtResponse.Body

	bodyBytes, err = json.Marshal(compileResponse)
	if err != nil {
		log.Printf("compileResponse marshal error: %v", err)
		http.Error(w, "Failed to encode data", http.StatusInternalServerError)
		return
	}

	w.Write(bodyBytes)
}

func (c *serverContext) gzHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/javascript")
	w.Header().Set("Content-Encoding", "gzip")
	http.ServeFile(w, r, gzPath(r.URL.Path))
	return
}

func (c *serverContext) shareHandler(w http.ResponseWriter, r *http.Request) {
	defer r.Body.Close()

	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	bodyBytes, err := c.runShare(r.Body)
	if err != nil {
		http.Error(w, "Failed to send share request", http.StatusInternalServerError)
		return
	}

	w.Write(bodyBytes)
}

func (c *serverContext) loadHandler(w http.ResponseWriter, r *http.Request) {
	defer r.Body.Close()

	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	response, err := http.Get(strings.Join([]string{c.backend, "p", r.URL.RawQuery + ".go"}, "/"))
	if err != nil {
		http.Error(w, "Failed to load snippet", http.StatusInternalServerError)
		return
	}
	defer response.Body.Close()

	bodyBytes, err := ioutil.ReadAll(response.Body)
	if err != nil {
		http.Error(w, "Failed to read data", http.StatusInternalServerError)
	}

	if response.StatusCode != http.StatusOK {
		http.Error(w, string(bodyBytes), response.StatusCode)
		return
	}

	w.Write(bodyBytes)
}
