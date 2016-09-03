package main

import (
	"fmt"
	"log"
	"net/http"
	"runtime"
	"sync"

	"golang.org/x/net/html"
)

type result struct {
	url  string
	name string
}

var fetched = struct {
	m map[string]error
	sync.Mutex
}{m: make(map[string]error)}

func fetch(url string) (*html.Node, error) {
	res, err := http.Get(url)
	if err != nil {
		log.Println(err)
		return nil, err
	}

	doc, err := html.Parse(res.Body)
	if err != nil {
		log.Println(err)
		return nil, err
	}

	return doc, nil
}

func parseFollowing(doc *html.Node, urls chan string) <-chan string {
	name := make(chan string)

	go func() {
		var f func(*html.Node)
		f = func(n *html.Node) {
			if n.Type == html.ElementNode && n.Data == "img" {
				for _, a := range n.Attr {
					if a.Key == "class" && a.Val == "avatar float-left" {
						for _, a := range n.Attr {
							if a.Key == "alt" {
								name <- a.Val
								break
							}
						}
					}

					if a.Key == "class" && a.Val == "avatar" {
						username := n.Parent.Attr[0].Val // get href value
						urls <- "https://github.com" + username + "/following"
						break
					}
				}
			}
			for c := n.FirstChild; c != nil; c = c.NextSibling {
				f(c)
			}
		}
		f(doc)
	}()

	return name
}

func crawl(url string, urls chan string, c chan<- result) {
	fetched.Lock()
	if _, ok := fetched.m[url]; ok { // check duplication
		fetched.Unlock()
		return
	}
	fetched.Unlock()

	doc, err := fetch(url)
	if err != nil {
		go func(u string) {
			urls <- u
		}(url)
		return
	}

	fetched.Lock()
	fetched.m[url] = err
	fetched.Unlock()

	name := <-parseFollowing(doc, urls)
	c <- result{url, name}
}

func worker(n int, done <-chan struct{}, urls chan string, c chan<- result) {
	for url := range urls {
		select {
		case <-done:
			return
		default:
			crawl(url, urls, c)
		}
	}
}

func main() {
	numCPUs := runtime.NumCPU()
	runtime.GOMAXPROCS(numCPUs)

	urls := make(chan string)
	done := make(chan struct{})
	c := make(chan result)

	var wg sync.WaitGroup
	const numWorkers = 10

	wg.Add(numWorkers)

	for i := 0; i < numWorkers; i++ {
		go func(n int) {
			worker(n, done, urls, c)
			wg.Done()
		}(i)
	}

	go func() {
		wg.Wait()
		close(c)
	}()

	urls <- "https://github.com/mingrammer/following"

	count := 0

	for r := range c {
		fmt.Println(r.name)

		count++

		if count > 200 {
			close(done)
			break
		}
	}
}
