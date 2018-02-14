package main

import (
	"fmt"
	"io/ioutil"
	"log"
	"net/http"
	"path"
	"sync"
	"time"

	"github.com/alecthomas/kingpin"
	"github.com/buger/jsonparser"
)

var (
	numImageDownloaders = kingpin.Flag("downloaders", "Number of simultaneous image downloads").Default("10").Int()
	outPath             = kingpin.Arg("outpath", "Directory where images will be saved").Required().ExistingDir()
	client              = http.Client{Timeout: 30 * time.Second}
	minWidth            = kingpin.Flag("minwidth", "Minimum width for an image to be downloaded").Default("4000").Int64()
	subreddits          = kingpin.Arg("subreddits", "Subreddit urls on reddit").Required().Strings()
)

func main() {
	log.SetFlags(log.LstdFlags | log.Lshortfile)
	kingpin.Parse()
	imageToLoadCh := make(chan *imageLoadRequest)
	imageToSaveCh := make(chan *imageLoadRequest)
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
	url := fmt.Sprintf("http://reddit.com/r/%v/top/.json?limit=500&t=week", subreddit)
	apiB, err := httpGet(url)
	if err != nil {
		log.Panicf("Error loading the API: %v", err)
	}
	_, err2 := jsonparser.ArrayEach(apiB, func(v []byte, dataType jsonparser.ValueType, offset int, err error) {
		id := mustGetString(v, "data", "id")
		defer func() {
			r := recover()
			if r != nil {
				log.Printf("Cannot load image for %v: %v", id, r)
			}
		}()
		url = mustGetString(v, "data", "preview", "images", "[0]", "source", "url")
		height := mustGetInt(v, "data", "preview", "images", "[0]", "source", "height")
		width := mustGetInt(v, "data", "preview", "images", "[0]", "source", "width")
		if width < *minWidth {
			return
		}
		ratio := float64(width) / float64(height)
		if ratio < 1.5 || ratio > 2 {
			//log.Printf("Ignoring %v because ratio %v(%vx%v) is not allowed", url, ratio, height, width)
			return
		}
		log.Println(url)
		outCh <- &imageLoadRequest{
			imageURL:  url,
			imageID:   id,
			subreddit: subreddit,
		}
	}, "data", "children")
	if err2 != nil {
		log.Panicf("Error parsing api: %v\n%s", err, apiB)
	}
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
