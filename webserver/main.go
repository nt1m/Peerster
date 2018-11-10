package webserver

import (
  "net"
  "net/http"
  "encoding/json"
  "fmt"
  "io"
  "bytes"
  "github.com/gorilla/mux"
  "github.com/dedis/protobuf"
  . "github.com/nt1m/Peerster/types"
)

type ReturnedFile struct {
  Name string
  Hash string
}

var gossiper *Gossiper
var port string

func NewWebServer(p string, g *Gossiper) {
  gossiper = g
  port = p

  router := mux.NewRouter()
  router.HandleFunc("/message", MessageGetHandler).Methods("GET")
  router.HandleFunc("/message", MessagePostHandler).Methods("POST")

  router.HandleFunc("/destination", DestinationGetHandler).Methods("GET")

  router.HandleFunc("/node", NodeGetHandler).Methods("GET")
  router.HandleFunc("/node", NodePostHandler).Methods("POST")

  router.HandleFunc("/file", FileGetHandler).Methods("GET")

  router.HandleFunc("/id", IdGetHandler).Methods("GET")
  router.PathPrefix("/").Handler(http.FileServer(http.Dir("./static/")))

  http.Handle("/", router)
  fmt.Println("Serving web server at:", port)
  http.ListenAndServe(":" + port, nil)
}

func MessageGetHandler(w http.ResponseWriter, r *http.Request) {
  w.WriteHeader(http.StatusOK)
  w.Header().Set("Content-Type", "application/json")

  str := "["
  i := 0
  for _, message := range gossiper.VisibleMessages {
    if i > 0 {
      str += ","
    }
    str += message.ToJSON()
    i++
  }
  str += "]"
  io.WriteString(w, str)
}

func MessagePostHandler(w http.ResponseWriter, r *http.Request) {
  decoder := json.NewDecoder(r.Body)
  var msg Message
  err := decoder.Decode(&msg)
  packetBytes, err := protobuf.Encode(&msg)
  FailIfErr(w, http.StatusBadRequest, err)
  conn, err := net.Dial("udp4", "127.0.0.1:" + port)
  FailIfErr(w, http.StatusBadRequest, err)
  if err == nil {
    conn.Write(packetBytes)
    w.WriteHeader(http.StatusOK)
  }
}

func DestinationGetHandler(w http.ResponseWriter, r *http.Request) {
  w.WriteHeader(http.StatusOK)
  w.Header().Set("Content-Type", "application/json")
  destinations := make([]string, 0, len(gossiper.Router))
  for destination := range gossiper.Router {
    destinations = append(destinations, destination)
  }

  json, err := json.Marshal(&destinations)
  FailIfErr(w, http.StatusInternalServerError, err)
  io.WriteString(w, string(json))
}

func NodeGetHandler(w http.ResponseWriter, r *http.Request) {
  w.WriteHeader(http.StatusOK)
  w.Header().Set("Content-Type", "application/json")

  io.WriteString(w, gossiper.PeersAsJSON())
}

func NodePostHandler(w http.ResponseWriter, r *http.Request) {
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
  w.WriteHeader(http.StatusOK)
  w.Header().Set("Content-Type", "text/plain")
  io.WriteString(w, gossiper.Name)
}

func FileGetHandler(w http.ResponseWriter, r *http.Request) {
  w.WriteHeader(http.StatusOK)
  w.Header().Set("Content-Type", "application/json")

  list := make([]*ReturnedFile, 0, len(gossiper.Files));
  for hash, file := range gossiper.Files {
    list = append(list, &ReturnedFile{file.FileName, hash})
  }
  json, err := json.Marshal(list)
  FailIfErr(w, http.StatusInternalServerError, err)
  io.WriteString(w, string(json))
}

func FailIfErr(w http.ResponseWriter, statusCode int, err error) {
  if err != nil {
    w.WriteHeader(statusCode)
    return;
  }
}
