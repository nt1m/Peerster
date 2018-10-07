package types

import (
  "net"
  "fmt"
  "strings"
  "time"
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
  CurrentID uint32
  Messages map[string]map[uint32]*RumorMessage // Map[Origin -> Map[Identifier][RumorMessage]]
  Timeouts map[string](chan bool)
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
    CurrentID: 1,
    Messages: make(map[string]map[uint32]*RumorMessage),
    Timeouts: make(map[string](chan bool)),
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

func (gossiper* Gossiper) RandomPeer(exclude *net.UDPAddr) *net.UDPAddr {
  index := rand.Intn(len(gossiper.Peers))
  if exclude != nil {
    for gossiper.Peers[index].String() != exclude.String() {
      index = rand.Intn(len(gossiper.Peers))
    }
  }

  return gossiper.Peers[index]
}

func (gossiper* Gossiper) RecordMessage(rm *RumorMessage) {
  if (gossiper.Messages[rm.Origin] == nil) {
    gossiper.Messages[rm.Origin] = make(map[uint32]*RumorMessage)
  }
  gossiper.Messages[rm.Origin][rm.ID] = rm
}

func (gossiper* Gossiper) GetNextIDForOrigin(origin string) uint32 {
  return uint32(len(gossiper.Messages[origin]) + 1)
}

func (gossiper* Gossiper) GetMessage(origin string, id uint32) *RumorMessage {
  return gossiper.Messages[origin][id]
}

func (gossiper* Gossiper) IsNewRumor(rm *RumorMessage) bool {
  return gossiper.GetNextIDForOrigin(rm.Origin) == rm.ID
}

func (gossiper* Gossiper) GetStatusPacket() *StatusPacket {
  var wanted []PeerStatus
  for origin := range gossiper.Messages {
    nextId := gossiper.GetNextIDForOrigin(origin)
    wanted = append(wanted, PeerStatus{origin, nextId})
  }
  return &StatusPacket{
    Want: wanted,
  }
}

func (gossiper *Gossiper) GetNewMessageForPeer(peerPacket *StatusPacket) *RumorMessage {
  statusMap := peerPacket.ToMap()
  for origin := range gossiper.Messages {
    nextId := gossiper.GetNextIDForOrigin(origin)
    if statusMap[origin] == nextId - 1 {
      return gossiper.GetMessage(origin, nextId - 1)
    }
  }
  return nil
}

func (gossiper *Gossiper) PeerHasMessages(peerPacket *StatusPacket) bool {
  statusMap := peerPacket.ToMap()
  for origin, nextId := range statusMap {
    if gossiper.GetNextIDForOrigin(origin) == nextId - 1 {
      return true
    }
  }
  return false
}

func (gossiper *Gossiper) ForwardToAllPeers(sender *net.UDPAddr, packetBytes []byte) {
  for _, peer := range gossiper.Peers {
    // Don't send to the person who just sent to you
    if (peer.String() == sender.String()) {
      return;
    }
    gossiper.Conn.WriteToUDP(packetBytes, peer)
  }
}

func (gossiper *Gossiper) SendPacket(destination *net.UDPAddr, packet *GossipPacket) {
  packetBytes := EncodePacket(packet)

  gossiper.Conn.WriteToUDP(packetBytes, destination)
}

func (gossiper *Gossiper) MongerMessage(msg *RumorMessage, exclude *net.UDPAddr, isFlippedCoin bool) {
  // Forward message to random peer
  destination := gossiper.RandomPeer(exclude)
  gossiper.SendPacket(destination, &GossipPacket{nil, msg, nil})
  if isFlippedCoin {
    fmt.Println("FLIPPED COIN sending rumor to", destination.String())
  } else {
    fmt.Println("MONGERING with", destination.String())
  }
  gossiper.Timeouts[destination.String()] = utils.SetTimeout(func() {
    gossiper.CoinFlip(msg, exclude)
  }, time.Second)
}

func (gossiper *Gossiper) CoinFlip(msg *RumorMessage, exclude *net.UDPAddr) {
  // Pick a new random peer and start mongering
  if rand.Int() % 2 == 0 {
    gossiper.MongerMessage(msg, exclude, true)
  }
}
