package main

import (
	"fmt"
	"log"
	"net/http"
	"runtime"
	"sync"

	"golang.org/x/net/html"
)

var startURL = "https://github.com/mingrammer/following"

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

func parseFollowing(doc *html.Node) []string {
	var urls = make([]string, 0)

	var f func(*html.Node)
	f = func(n *html.Node) {
		if n.Type == html.ElementNode && n.Data == "img" {
			for _, a := range n.Attr {
				if a.Key == "class" && a.Val == "avatar float-left" {
					for _, a := range n.Attr {
						if a.Key == "alt" {
							fmt.Println(a.Val)
							break
						}
					}
				}

				if a.Key == "class" && a.Val == "avatar" {
					username := n.Parent.Attr[0].Val // get href value
					urls = append(urls, "https://github.com"+username+"/following")
					break
				}
			}
		}
		for c := n.FirstChild; c != nil; c = c.NextSibling {
			f(c)
		}
	}
	f(doc)

	return urls
}

func crawl(url string) {
	fetched.Lock()
	if _, ok := fetched.m[url]; ok { // check duplication
		fetched.Unlock()
		return
	}
	fetched.Unlock()

	doc, err := fetch(url)

	fetched.Lock()
	fetched.m[url] = err
	fetched.Unlock()

	urls := parseFollowing(doc)

	done := make(chan bool)

	for _, u := range urls {
		go func(url string) {
			crawl(url)
			done <- true
		}(u)
	}

	for i := 0; i < len(urls); i++ {
		<-done
	}
}

func main() {
	numCPUs := runtime.NumCPU()
	runtime.GOMAXPROCS(numCPUs)

	crawl(startURL)
}
