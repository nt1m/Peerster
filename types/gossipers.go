package types

import (
  "net"
  "fmt"
  "strings"
  "math/rand"
  "Peerster/utils"
)

type Client struct {
  Address *net.UDPAddr
  Conn *net.UDPConn
}

type Gossiper struct {
  Address *net.UDPAddr
  Conn *net.UDPConn
  Name string
  Peers []*net.UDPAddr
  Status map[string]uint32 // Map[Sender -> NextMsgID]
}

func NewClient(address string) *Client {
  udpAddr, err := net.ResolveUDPAddr("udp4", address)
  udpConn, err := net.ListenUDP("udp4", udpAddr)
  if (err != nil) {
    fmt.Println(err)
  }

  return &Client{
    Address: udpAddr,
    Conn: udpConn,
  }
}

func NewGossiper(address, name, peerStr string) *Gossiper {
  udpAddr, err := net.ResolveUDPAddr("udp4", address)
  udpConn, err := net.ListenUDP("udp4", udpAddr)
  if (err != nil) {
    fmt.Println(err)
  }

  peers := strings.Split(
    peerStr, ",")

  var peerAddrs []*net.UDPAddr
  for _,peer := range peers {
    peerAddr, err := net.ResolveUDPAddr("udp4", peer)
    utils.CheckError(err)
    peerAddrs = append(peerAddrs, peerAddr)
  }

  return &Gossiper{
    Address: udpAddr,
    Conn: udpConn,
    Name: name,
    Peers: peerAddrs,
    Status: make(map[string]uint32),
  }
}

func (gossiper* Gossiper) AddPeer(address *net.UDPAddr) {
  for _, peer := range gossiper.Peers {
    if peer.String() == address.String() {
      return
    }
  }
  gossiper.Peers = append(gossiper.Peers, address)
}

func (gossiper* Gossiper) PeersAsString() string {
  str := ""
  for i, peer := range gossiper.Peers {
    if i == 0 {
      str = peer.String()
    } else {
      str = str + "," + peer.String()
    }
  }
  return str
}

func (gossiper* Gossiper) RandomPeer() *net.UDPAddr {
  index := rand.Intn(len(gossiper.Peers))
  return gossiper.Peers[index]
}

func (gossiper* Gossiper) UpdateStatus(rm *RumorMessage) {
  gossiper.Status[rm.Origin] = rm.ID + 1
}

func (gossiper* Gossiper) IsNewRumor(rm *RumorMessage) bool {
  if gossiper.Status[rm.Origin] > rm.ID {
    fmt.Println("WARNING:", "skipped rumor ID", gossiper.Status[rm.Origin])
  }
  return gossiper.Status[rm.Origin] >= rm.ID
}
