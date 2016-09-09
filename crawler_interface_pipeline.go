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

// FollowingResult stores url and name of following account.
type FollowingResult struct {
	url  string
	name string
}

// StarsResult stores repo name of starred repository.
type StarsResult struct {
	repo string
}

// FetchedURL has map to determine if following url is processed already.
type FetchedURL struct {
	m map[string]error
	sync.Mutex
}

// FetchedRepo has map to determine if repo name is processed already.
type FetchedRepo struct {
	m map[string]struct{}
	sync.Mutex
}

// GitHubFollowing stores following information.
type GitHubFollowing struct {
	fetchedURL *FetchedURL          // shared fetched url map.
	p          *Pipeline            // shared pipeline.
	stars      *GitHubStars         // shared struct for storing starred repo information.
	result     chan FollowingResult // shared space for storing crawled following data.
	url        string               // url for crawling and parsing the following user.
}

// GitHubStars stores starred repository information.
type GitHubStars struct {
	fetchedURL  *FetchedURL      // shared fetched url map.
	fetchedRepo *FetchedRepo     // shared fetched repo name map.
	p           *Pipeline        // share pipeline.
	result      chan StarsResult // shared space for storing crawled starred repo data.
	url         string           // url for crawling and parsing the starred repository.
}

// Crawler interface.
type Crawler interface {
	Crawl()
}

// Pipeline is struct for crawling the github data concurrently.
type Pipeline struct {
	request chan Crawler
	done    chan struct{}
	wg      *sync.WaitGroup
}

// fetch parses the url and return parsed data.
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

// Request of GitHubFollowing sends the request for crawling following to requests channel of pipeline.
func (g *GitHubFollowing) Request(url string) {
	g.p.request <- &GitHubFollowing{
		fetchedURL: g.fetchedURL,
		p:          g.p,
		stars:      g.stars,
		result:     g.result,
		url:        url,
	}
}

// Request of GitHubStars sends the request for crawling starred repo to requests channel of pipeline.
func (g *GitHubStars) Request(url string) {
	g.p.request <- &GitHubStars{
		fetchedURL:  g.fetchedURL,
		fetchedRepo: g.fetchedRepo,
		p:           g.p,
		result:      g.result,
		url:         url,
	}
}

// Parse of GitHubFollowing parses the following page.
func (g *GitHubFollowing) Parse(doc *html.Node) <-chan string {
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
						username := n.Parent.Attr[0].Val // get href value.

						g.Request("https://github.com" + username + "/following")
						g.stars.Request("https://github.com/stars" + username)

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

// Parse of GitHubStars parses the page of starred repositories list.
func (g *GitHubStars) Parse(doc *html.Node) <-chan string {
	repo := make(chan string)

	go func() {
		defer close(repo) // Close the channel before goroutine will be stopped.

		var f func(*html.Node)
		f = func(n *html.Node) {
			if n.Type == html.ElementNode && n.Data == "span" {
				for _, a := range n.Attr {
					if a.Key == "class" && a.Val == "prefix" {
						repo <- n.Parent.Attr[0].Val
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

	return repo
}

// Crawl of GitHubFollowing crawls the following url (is not processed already) and receives the results.
func (g *GitHubFollowing) Crawl() {
	g.fetchedURL.Lock()
	if _, ok := g.fetchedURL.m[g.url]; ok {
		g.fetchedURL.Unlock()
		return
	}
	g.fetchedURL.Unlock()

	doc, err := fetch(g.url)
	if err != nil {
		go func(u string) {
			g.Request(u)
		}(g.url)
		return
	}

	g.fetchedURL.Lock()
	g.fetchedURL.m[g.url] = err
	g.fetchedURL.Unlock()

	name := <-g.Parse(doc)
	g.result <- FollowingResult{g.url, name}
}

// Crawl of GitHubStars crawls the starred repositories url (is not processed already) and receives the results.
func (g *GitHubStars) Crawl() {
	g.fetchedURL.Lock()
	if _, ok := g.fetchedURL.m[g.url]; ok {
		g.fetchedURL.Unlock()
		return
	}
	g.fetchedURL.Unlock()

	doc, err := fetch(g.url)
	if err != nil {
		go func(u string) {
			g.Request(u)
		}(g.url)
		return
	}

	g.fetchedURL.Lock()
	g.fetchedURL.m[g.url] = err
	g.fetchedURL.Unlock()

	// Receives the every starred repositories.
	repositories := g.Parse(doc)
	for r := range repositories {
		g.fetchedRepo.Lock()
		if _, ok := g.fetchedRepo.m[r]; !ok {
			g.result <- StarsResult{r}
			g.fetchedRepo.m[r] = struct{}{}
		}
		g.fetchedRepo.Unlock()
	}
}

// NewPipeline creates the new pipeline instance.
func NewPipeline() *Pipeline {
	return &Pipeline{
		request: make(chan Crawler),
		done:    make(chan struct{}),
		wg:      new(sync.WaitGroup),
	}
}

// Run makes multiple workers and waits until workers are stopped all.
func (p *Pipeline) Run() {
	const numWorkers = 10
	p.wg.Add(numWorkers)
	for i := 0; i < numWorkers; i++ {
		go func() {
			p.Worker()
			p.wg.Done()
		}()
	}

	go func() {
		p.wg.Wait()
	}()
}

// Worker executes the crawling job until done signal is arrived.
func (p *Pipeline) Worker() {
	for r := range p.request {
		select {
		case <-p.done:
			return
		default:
			r.Crawl()
		}
	}
}

func main() {
	numCPUs := runtime.NumCPU()
	runtime.GOMAXPROCS(numCPUs)

	p := NewPipeline()
	p.Run()

	// Initializes the stars and following with intial url.
	stars := &GitHubStars{
		fetchedURL:  &FetchedURL{m: make(map[string]error)},
		fetchedRepo: &FetchedRepo{m: make(map[string]struct{})},
		p:           p,
		result:      make(chan StarsResult),
	}
	following := &GitHubFollowing{
		fetchedURL: &FetchedURL{m: make(map[string]error)},
		p:          p,
		stars:      stars,
		result:     make(chan FollowingResult),
		url:        startURL,
	}

	p.request <- following

	count := 0

LOOP:
	for {
		select {
		case f := <-following.result:
			fmt.Println(f.name)
		case s := <-stars.result:
			fmt.Println(s.repo)

			// Crawls the urls only 1000.
			if count == 100 {
				close(p.done) // Stops the all workers.
				break LOOP
			}

			count++
		}
	}
}
