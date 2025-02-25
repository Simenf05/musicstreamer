package main

import (
	"fmt"
	"io"
	"log"
	"net/http"
	"regexp"
	"sync"
	"time"
)

type Downloader struct {
    output chan []byte
    stop chan bool
}

func (downloader *Downloader) readAndSend(resp *http.Response, buf []byte ) error {
    length, err := resp.Body.Read(buf)
    if err != nil {
        return err
    }

    downloader.output<-buf[:length]
    return nil
}

func (downloader *Downloader) download(url string) {
    resp, err := http.Get(url)
    if err != nil {
        log.Fatal(err)
    }
    defer resp.Body.Close()

    buf := make([]byte, 8192)
    ticker := time.NewTicker(150 * time.Millisecond)

    for range ticker.C {
        select {
            case <-downloader.stop:
                break
            default:
            err := downloader.readAndSend(resp, buf)
            if err != nil {
                log.Printf("Downloader stopped with: %s", err)
                break
            }
        }
    }
}

type Connection struct {
    id int
    buffer chan []byte
    stop chan bool
}

type ConnectionContainer struct {
    connections map[Connection]struct{}
    mu sync.Mutex
}

func (container *ConnectionContainer) newConnection(id int) *Connection {
    defer container.mu.Unlock()
    container.mu.Lock()

    newConn := Connection{buffer: make(chan []byte, 100), id: id}
    container.connections[newConn] = struct{}{}

    log.Printf("Starting connection with id: %d", id)

    return &newConn
}

func (container *ConnectionContainer) removeConnection(connection *Connection) {
    defer container.mu.Unlock()
    container.mu.Lock()

    if _, ok := container.connections[*connection]; ok {
        log.Printf("Removing connection with id: %d", connection.id)
        delete(container.connections, *connection)
    }
}


func main() {
    log.Println("Starting the server.")

    container := &ConnectionContainer{
        connections: make(map[Connection]struct{}),
    }

    http.HandleFunc("/search", func(w http.ResponseWriter, r *http.Request) {

        url := "https://ncs.io/music-search?q=%s&genre=%s&mood=%s"

        name := r.URL.Query().Get("name")
        genre := r.URL.Query().Get("genre")
        mood := r.URL.Query().Get("mood")

        url = fmt.Sprintf(url, name, genre, mood)
        
        resp, err := http.Get(url)
        if err != nil {
            log.Fatal(err)
        }
        defer resp.Body.Close()

        body, err := io.ReadAll(resp.Body)
        if err != nil {
            log.Fatal(err)
        }

        // data-url="https://ncsmusic.s3.eu-west-1.amazonaws.com/tracks/000/001/127/on-on-nuumi-remix-1652349640-5EszNOOTne.mp3"
        // in the html this is the grep for url of all the songs relevant to the search
        regexpattern := regexp.MustCompile(`data-url="https:\/\/ncsmusic\.s3\.eu-west-1\.amazonaws\.com\/tracks\/\d{3}\/\d{3}\/\d{3}\/[\w\-]+\.mp3"`)

        matches := regexpattern.FindAll(body, -1)

        log.Printf("%q\n", matches)

        w.Write()

    })

    nextId := 0
    http.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
        url := "https://ncsmusic.s3.eu-west-1.amazonaws.com/tracks/000/001/759/invincible-sped-up-1727366457-RfLWTl9BNp.mp3"

        w.Header().Add("Content-Type", "audio/mpeg")
        w.Header().Add("Connection", "keep-alive")

        flusher, ok := w.(http.Flusher)
        if !ok {
            log.Println(ok)
            return
        }

        conn := container.newConnection(nextId)
        nextId += 1

        downloader := Downloader{
            output: conn.buffer,
            stop: make(chan bool, 10),
        }

        go downloader.download(url)

        // go conn.stream(url)

        for {
            message := <-conn.buffer

            _, err := w.Write(message)
            if err != nil {
                log.Printf("Stopping connection with id: %d with error: %s", conn.id, err)
                container.removeConnection(conn)
                downloader.stop <- true
                return
            }

            flusher.Flush()
        }

    })

    log.Fatal(http.ListenAndServe(":8080", nil))
}



