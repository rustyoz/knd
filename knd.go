package main

import (
	"fmt"
	"io"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"golang.org/x/net/html"
)

// Helper function to pull the href attribute from a Token
func getHref(t html.Token) (ok bool, href string) {
	// Iterate over all of the Token's attributes until we find an "href"
	for _, a := range t.Attr {
		if a.Key == "href" {
			href = a.Val
			ok = true
		}
	}

	// "bare" return will return the variables (ok, href) as defined in
	// the function definition
	return
}

// Extract all http** links from a given webpage
func crawl(url string, ch chan string, chFinished chan bool) {
	resp, err := http.Get(url)

	defer func() {
		// Notify that we're done after this function
		chFinished <- true
	}()

	if err != nil {
		fmt.Println("ERROR: Failed to crawl \"" + url + "\"")
		return
	}

	b := resp.Body
	defer b.Close() // close Body when the function returns

	z := html.NewTokenizer(b)

	for {
		tt := z.Next()

		switch {
		case tt == html.ErrorToken:
			// End of the document, we're done
			return
		case tt == html.StartTagToken:
			t := z.Token()

			// Check if the token is an <a> tag
			isAnchor := t.Data == "a"
			if !isAnchor {
				continue
			}

			// Extract the href value, if there is one
			ok, url := getHref(t)
			if !ok {
				continue
			}

			// Make sure the url begines in http**
			hasProto := strings.Index(url, "http") == 0
			if hasProto {
				ch <- url
			}
		}
	}
}

func main() {
	c := make(chan os.Signal, 1)
	signal.Notify(c, os.Interrupt)
	signal.Notify(c, syscall.SIGINT)

	sourceurl := "http://downloads.kicad-pcb.org/windows/nightly/"

	if len(os.Args) != 2 {
		fmt.Println("Usage: knd download/directory")
		return
	}
	downloadto := os.Args[1]
	go func() {
		for {
			var lastdownload string
			newdownload, err := checknightlybuilds(sourceurl, downloadto)
			if err != nil {
				fmt.Println(err)
				os.Exit(1)
			}
			if lastdownload != "" {
				os.Remove(lastdownload)
			}
			lastdownload = newdownload
			time.Sleep(time.Hour)
		}
	}()

	<-c
	fmt.Println("Ctrl-C Detected Stopping")
	os.Exit(1)
}

func checknightlybuilds(sourceurl string, downloadto string) (string, error) {
	resp, err := http.Get(sourceurl)
	if err != nil {
		return "", err
	}

	b := resp.Body
	//htmlData, err := ioutil.ReadAll(resp.Body) //<--- here!

	if err != nil {
		fmt.Println(err)
		os.Exit(1)
	}

	defer b.Close()

	z := html.NewTokenizer(b)

	var atokens []html.Token

Loop:
	for {
		tt := z.Next()
		switch {
		case tt == html.ErrorToken:
			// End of the document, we're done
			break Loop
		case tt == html.StartTagToken:
			t := z.Token()
			// Check if the token is an <a> tag
			isAnchor := t.Data == "a"
			if !isAnchor {
				continue
			} else {

				atokens = append(atokens, t)
			}
		}
	}

	lasttoken := atokens[len(atokens)-1]
	ok, lastlink := getHref(lasttoken)
	fmt.Println(lastlink)
	if !ok {
		return "", fmt.Errorf("%s", "getHref failed")
	}
	if _, err := os.Stat(lastlink); os.IsNotExist(err) {
		err = downloadFile(downloadto+lastlink, sourceurl+lastlink)
		if err != nil {
			return "", fmt.Errorf("Download Failed %s", err)
		}
	}
	return downloadto + lastlink, nil
}

func downloadFile(filepath string, url string) (err error) {

	// Create the file
	out, err := os.Create(filepath)
	if err != nil {
		return err
	}
	defer out.Close()

	// Get the data
	fmt.Println("Downloading:", url)

	response, err := http.Get(url)
	readerpt := &PassThru{Reader: response.Body, length: response.ContentLength}
	if err != nil {
		return err
	}
	defer response.Body.Close()

	if err != nil {
		return fmt.Errorf("Error while reading downloaded", url, "-", err)
	}

	// Writer the body to file
	_, err = io.Copy(out, readerpt)
	if err != nil {
		return fmt.Errorf("Error while reading downloaded", url, "-", err)
	}

	return nil
}

type PassThru struct {
	io.Reader
	total    int64 // Total # of bytes transferred
	length   int64 // Expected length
	progress float64
}

// Read 'overrides' the underlying io.Reader's Read method.
// This is the one that will be called by io.Copy(). We simply
// use it to keep track of byte counts and then forward the call.
func (pt *PassThru) Read(p []byte) (int, error) {
	n, err := pt.Reader.Read(p)
	if n > 0 {
		pt.total += int64(n)
		percentage := float64(pt.total) / float64(pt.length) * float64(100)
		i := int(percentage / float64(10))
		is := fmt.Sprintf("%v", i)
		if percentage-pt.progress > 2 {
			fmt.Fprintf(os.Stderr, is)
			pt.progress = percentage
		}
	}

	return n, err
}

type byModTime []os.FileInfo

func (b byModTime) Len() int {
	return len(b)
}

func (b byModTime) Less(i, j int) bool {
	return b[i].ModTime().Before(b[j].ModTime())
}

func (b byModTime) Swap(i, j int) {
	b[i], b[j] = b[j], b[i]
}
