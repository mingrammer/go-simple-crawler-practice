# go-simple-crawler-practice
Simple crawler for learning go
> What can you learn from this simple codes is the simple go program structures and the little understanding about concurrency.

<br>

## Description
* **crawler.go**

    > Crawls the following of users on github recursively.

* **crawler_pipeline.go**
    
    > Crawls the following of users using workers concurrently.

* **crawler_interface_pipeline.go**
    
    > Crawls the following and starred repositories of users using crwaler interface and workers concurrently.

<br>

## Run
1. Modify the `startURL` at top of file to fit your account
2. `go run <crawler-file>`
