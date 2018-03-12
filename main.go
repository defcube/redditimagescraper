package main

import (
	"fmt"
	"html"
	"io/ioutil"
	"log"
	"net/http"
	"path"
	"sync"
	"time"

	"github.com/alecthomas/kingpin"
	"github.com/buger/jsonparser"
	_ "net/http/pprof"
	"net"
)

var (
	numImageDownloaders = kingpin.Flag("downloaders", "Number of simultaneous image downloads").Default("10").Int()
	outPath             = kingpin.Arg("outpath", "Directory where images will be saved").Required().ExistingDir()
	client              *http.Client
	minWidth            = kingpin.Flag("minwidth", "Minimum width for an image to be downloaded").Default("4000").Int64()
	minHeight           = kingpin.Flag("minheight", "Minimum height for an image to be downloaded").Default("2000").Int64()
	subreddits          = kingpin.Arg("subreddits", "Subreddit urls on reddit").Required().Strings()
	maxPerSubreddit     = kingpin.Flag("maxpersubreddit", "Max images to download per subreddit").Default("10").Int()
	debugPort           = kingpin.Flag("debugserver", "Port on which to run a debug http server").Int()
)

func main() {
	log.SetFlags(log.LstdFlags | log.Lshortfile)
	kingpin.Parse()

	client = &http.Client{Timeout: 30 * time.Second, Transport: &http.Transport{
		Proxy: http.ProxyFromEnvironment,
		DialContext: (&net.Dialer{
			Timeout:   60 * time.Second,
			KeepAlive: 30 * time.Second,
			DualStack: true,
		}).DialContext,
		MaxIdleConns:          *numImageDownloaders,
		MaxIdleConnsPerHost:   *numImageDownloaders,
		IdleConnTimeout:       90 * time.Second,
		TLSHandshakeTimeout:   10 * time.Second,
		ExpectContinueTimeout: 1 * time.Second,
	}}

	// imageToLoadCh has a large buffer to prevent the loadAPI from starving the rest of the api if it's scanning
	// forums that have nothing
	imageToLoadCh := make(chan *imageLoadRequest, *numImageDownloaders*20)
	imageToSaveCh := make(chan *imageLoadRequest)

	if *debugPort != 0 {
		log.Printf("Launching debug server on %v", *debugPort)
		go func() {
			log.Println(http.ListenAndServe(fmt.Sprintf("0.0.0.0:%v", *debugPort), nil))
		}()

	}

	go func() {
		for _, subreddit := range *subreddits {
			loadAPI(subreddit, imageToLoadCh)
		}
		close(imageToLoadCh)
	}()
	go func() {
		wg := sync.WaitGroup{}
		wg.Add(*numImageDownloaders)
		for i := 0; i < *numImageDownloaders; i++ {
			go func() {
				var err error
				for req := range imageToLoadCh {
					req.data, err = httpGet(req.imageURL)
					if err != nil {
						log.Printf("Error getting image %v: %v", req.imageURL, err)
						continue
					}
					imageToSaveCh <- req
				}

				wg.Done()
			}()
		}
		wg.Wait()
		close(imageToSaveCh)
	}()
	for r := range imageToSaveCh {
		fp := path.Join(*outPath, fmt.Sprint(r.subreddit, "-", r.imageID, ".jpg"))
		if err := ioutil.WriteFile(fp, r.data, 0644); err != nil {
			log.Panicf("Cannot save file %v: %v", fp, err)
		}
	}
}

type imageLoadRequest struct {
	subreddit string
	imageURL  string
	imageID   string
	data      []byte
}

func loadAPI(subreddit string, outCh chan *imageLoadRequest) {
	lastID := ""
	baseURL := "http://reddit.com/r/%v/top/.json?limit=100&t=month"
	baseURL = fmt.Sprintf(baseURL, subreddit)
	imageCount := 0
	for i := 0; i < 5; i++ {
		var url string
		if lastID != "" {
			url = baseURL + "&after=t3_" + lastID
		} else {
			url = baseURL
		}
		apiB, err := httpGet(url)
		if err != nil {
			log.Panicf("Error loading the API: %v", err)
		}
		_, err2 := jsonparser.ArrayEach(apiB, func(v []byte, dataType jsonparser.ValueType, offset int, err error) {
			if imageCount == *maxPerSubreddit {
				return
			}
			lastID = mustGetString(v, "data", "id")
			defer func() {
				recover() // nolint
				// don't check the recover result, just assum it's due to mustGetString or one of those calls,
				// which indicates that there isn't a photo
			}()
			url = mustGetString(v, "data", "preview", "images", "[0]", "source", "url")
			height := mustGetInt(v, "data", "preview", "images", "[0]", "source", "height")
			width := mustGetInt(v, "data", "preview", "images", "[0]", "source", "width")
			if width < *minWidth || height < *minHeight {
				return
			}

			// shouldn't have to do unescape, but reddit introduced what seems to be a bug on 2/14/18
			url = html.UnescapeString(url)

			outCh <- &imageLoadRequest{
				imageURL:  url,
				imageID:   lastID,
				subreddit: subreddit,
			}
			imageCount++
		}, "data", "children")
		if err2 != nil {
			log.Panicf("Error parsing api: %v\n%s", err, apiB)
		}
		if imageCount == *maxPerSubreddit {
			break
		}
	}
	log.Printf("Found %v photo(s) on /r/%v", imageCount, subreddit)
}

func mustGetString(v []byte, path ...string) string {
	r, err := jsonparser.GetString(v, path...)
	if err != nil {
		panic(err)
	}
	return r
}

func mustGetInt(v []byte, path ...string) int64 {
	r, err := jsonparser.GetInt(v, path...)
	if err != nil {
		panic(err)
	}
	return r
}

func httpGet(url string) (data []byte, err error) {
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		panic(err)
	}
	req.Header.Set("User-Agent", "redditimagescraper")
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer func() {
	}()
	b, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}
	err = resp.Body.Close()
	if err != nil {
		return nil, fmt.Errorf("resp.Body.Close: %v", err)
	}
	return b, nil
}
