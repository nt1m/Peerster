package webserver

import (
  "net"
  "net/http"
  "fmt"
  "io"
  "bytes"
  "github.com/gorilla/mux"
  "github.com/dedis/protobuf"
  . "Peerster/types"
)

var gossiper *Gossiper
var port string

func NewWebServer(p string, g *Gossiper) {
  gossiper = g
  port = p

  router := mux.NewRouter()
  router.HandleFunc("/message", MessageGetHandler).Methods("GET")
  router.HandleFunc("/message", MessagePostHandler).Methods("POST")

  router.HandleFunc("/node", NodeGetHandler).Methods("GET")
  router.HandleFunc("/node", NodePostHandler).Methods("POST")

  router.HandleFunc("/id", IdGetHandler).Methods("GET")
  router.PathPrefix("/").Handler(http.FileServer(http.Dir("./static/")))

  http.Handle("/", router)
  fmt.Println("Serving web server at:", port)
  http.ListenAndServe(":" + port, nil)
}

func MessageGetHandler(w http.ResponseWriter, r *http.Request) {
  fmt.Println("GET /message")
  w.WriteHeader(http.StatusOK)
  w.Header().Set("Content-Type", "application/json")

  str := "["
  i := 0
  for _, msgByID := range gossiper.Messages {
    for _, message := range msgByID {
      if i > 0 {
        str += ","
      }
      str += message.ToJSON()
      i++
    }
  }
  str += "]"
  io.WriteString(w, str)
}

func MessagePostHandler(w http.ResponseWriter, r *http.Request) {
  fmt.Println("POST /message")
  buf := new(bytes.Buffer)
  buf.ReadFrom(r.Body)
  str := buf.String()
  msg := Message{Text: str}
  packetBytes, err := protobuf.Encode(&msg)
  FailIfErr(w, http.StatusBadRequest, err)
  conn, err := net.Dial("udp4", "127.0.0.1:" + port)
  FailIfErr(w, http.StatusBadRequest, err)
  if err == nil {
    conn.Write(packetBytes)
    w.WriteHeader(http.StatusOK)
  }
}

func NodeGetHandler(w http.ResponseWriter, r *http.Request) {
  fmt.Println("GET /node")
  w.WriteHeader(http.StatusOK)
  w.Header().Set("Content-Type", "application/json")

  io.WriteString(w, gossiper.PeersAsJSON())
}

func NodePostHandler(w http.ResponseWriter, r *http.Request) {
  fmt.Println("POST /node")
  buf := new(bytes.Buffer)
  buf.ReadFrom(r.Body)
  str := buf.String()

  udpAddr, err := net.ResolveUDPAddr("udp4", str)
  FailIfErr(w, http.StatusBadRequest, err)

  if err == nil {
    gossiper.AddPeer(udpAddr)
    w.WriteHeader(http.StatusOK)
  }
}

func IdGetHandler(w http.ResponseWriter, r *http.Request) {
  fmt.Println("GET /id")
  w.WriteHeader(http.StatusOK)
  w.Header().Set("Content-Type", "text/plain")
  io.WriteString(w, gossiper.Name)
}

func FailIfErr(w http.ResponseWriter, statusCode int, err error) {
  if err != nil {
    w.WriteHeader(statusCode)
    return;
  }
}
