package main

import (
    "bytes"
	"io"
	"log"
	"net/http"
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
    buffer chan []byte
    stop chan bool
}



func (conn *Connection) stream(url string) {

    go downloadFile(url, conn.buffer)
    ticker := time.NewTicker(150 * time.Millisecond)

    for range ticker.C {
        select {
            case <-conn.stop:
                return
            default:
                continue
        }
    }
}

type ConnectionContainer struct {
    connections map[Connection]struct{}
    mu sync.Mutex
}

func (container *ConnectionContainer) newConnection() *Connection {
    defer container.mu.Unlock()
    container.mu.Lock()

    newConn := Connection{buffer: make(chan []byte, 100)}
    container.connections[newConn] = struct{}{}

    log.Println("Starting connection")

    return &newConn
}

func (container *ConnectionContainer) removeConnection(connection *Connection) {
    defer container.mu.Unlock()
    container.mu.Lock()

    if _, ok := container.connections[*connection]; ok {
        log.Println("Removing connection")
        delete(container.connections, *connection)
    }
}

func (connContainer *ConnectionContainer) distributer (music chan []byte) {
    ticker := time.NewTicker(150 * time.Millisecond)
    tempBuffer := make([]byte, 8192)
    tempFile := bytes.NewReader(<-music)

    for range ticker.C {
        tempFile.Read(tempBuffer)
        for conn := range connContainer.connections {
            conn.buffer <- tempBuffer
        }
    }
}

func downloadFile(url string, music chan []byte) {

    resp, err := http.Get(url)
    if err != nil {
        log.Fatal(err)
    }

    defer resp.Body.Close()
    body, err := io.ReadAll(resp.Body)
    if err != nil {
        log.Fatal(err)
    }

    music <- body
}


func main() {

    container := &ConnectionContainer{
        connections: make(map[Connection]struct{}),
    }

    // music := make(chan []byte)

    url := "https://ncsmusic.s3.eu-west-1.amazonaws.com/tracks/000/001/759/invincible-sped-up-1727366457-RfLWTl9BNp.mp3"
    // go downloadFile(url, music)
    // go container.distributer(music)

    http.HandleFunc("/search", func(w http.ResponseWriter, r *http.Request) {

        // http.Get(w)

    })

    http.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {

        w.Header().Add("Content-Type", "audio/mpeg")
        w.Header().Add("Connection", "keep-alive")

        flusher, ok := w.(http.Flusher)
        if !ok {
            log.Fatal(ok)
            // log.Println(ok)
            // return
        }

        conn := container.newConnection()

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
                log.Printf("Stopping connection with error: %s", err)
                container.removeConnection(conn)
                downloader.stop <- true
                return
            }

            flusher.Flush()
        }

    })

    log.Fatal(http.ListenAndServe(":8080", nil))
}



