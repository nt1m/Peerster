package types

import (
  "net"
  "fmt"
  "strings"
  "time"
  "math/rand"
  "github.com/nt1m/Peerster/utils"
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
  Rumors map[string]map[uint32]*RumorMessage // Map[Origin -> Map[Identifier][RumorMessage]]
  VisibleMessages []*GossipPacket
  Router map[string]*net.UDPAddr // Map[Origin -> UDPAddr]
  Timeouts map[string](chan bool)
  LastRumor map[string]*RumorMessage
  LastInteraction *net.UDPAddr
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
    Rumors: make(map[string]map[uint32]*RumorMessage),
    Router: make(map[string]*net.UDPAddr),
    Timeouts: make(map[string](chan bool)),
    LastRumor: make(map[string]*RumorMessage),
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
      str += "," + peer.String()
    }
  }
  return str
}

func (gossiper* Gossiper) PeersAsJSON() string {
  if (len(gossiper.Peers) == 0) {
    return "[]";
  }

  str := "["
  for i, peer := range gossiper.Peers {
    if i == 0 {
      str += `"` + peer.String() + `"`
    } else {
      str += "," + `"` + peer.String() + `"`
    }
  }
  str += "]"
  return str
}

func (gossiper* Gossiper) RandomPeer(exclude *net.UDPAddr) *net.UDPAddr {
  rand.Seed(time.Now().UnixNano())
  index := rand.Intn(len(gossiper.Peers))
  if exclude != nil && len(gossiper.Peers) > 1 {
    for gossiper.Peers[index].String() == exclude.String() {
      index = rand.Intn(len(gossiper.Peers))
    }
    utils.Assert(gossiper.Peers[index].String() != exclude.String())
  }
  return gossiper.Peers[index]
}

func (gossiper* Gossiper) RecordRumor(rm *RumorMessage) {
  if (gossiper.Rumors[rm.Origin] == nil) {
    gossiper.Rumors[rm.Origin] = make(map[uint32]*RumorMessage)
  }
  gossiper.Rumors[rm.Origin][rm.ID] = rm
  if (rm.Text != "") {
    gossiper.VisibleMessages = append(gossiper.VisibleMessages, &GossipPacket{nil, rm, nil, nil})
  }
}

func (gossiper* Gossiper) RecordPrivate(msg *PrivateMessage) {
  gossiper.VisibleMessages = append(gossiper.VisibleMessages, &GossipPacket{nil, nil, nil, msg})
}

func (gossiper* Gossiper) ForwardPrivate(pm *PrivateMessage) {
  if pm.HopLimit > 0 {
    gossiper.SendPacket(
      gossiper.Router[pm.Destination],
      &GossipPacket{nil, nil, nil, pm})
  }
}

func (gossiper* Gossiper) GetNextIDForOrigin(origin string) uint32 {
  return uint32(len(gossiper.Rumors[origin]) + 1)
}

func (gossiper* Gossiper) GetMessage(origin string, id uint32) *RumorMessage {
  return gossiper.Rumors[origin][id]
}

func (gossiper* Gossiper) ShouldIgnoreRumor(rm *RumorMessage) bool {
  return gossiper.GetNextIDForOrigin(rm.Origin) < rm.ID
}

func (gossiper* Gossiper) IsNewRumor(rm *RumorMessage) bool {
  return gossiper.GetNextIDForOrigin(rm.Origin) == rm.ID
}

func (gossiper* Gossiper) GetStatusPacket() *StatusPacket {
  var wanted []PeerStatus
  for origin := range gossiper.Rumors {
    nextId := gossiper.GetNextIDForOrigin(origin)
    wanted = append(wanted, PeerStatus{origin, nextId})
  }
  return &StatusPacket{
    Want: wanted,
  }
}

func (gossiper *Gossiper) GetNewRumorForPeer(peerPacket *StatusPacket) *RumorMessage {
  statusMap := peerPacket.ToMap()
  for origin := range gossiper.Rumors {
    nextId := gossiper.GetNextIDForOrigin(origin)
    if statusMap[origin] == nextId - 1 {
      return gossiper.GetMessage(origin, nextId - 1)
    }
  }
  return nil
}

func (gossiper *Gossiper) PeerHasRumors(peerPacket *StatusPacket) bool {
  statusMap := peerPacket.ToMap()
  for origin, nextId := range statusMap {
    if gossiper.GetNextIDForOrigin(origin) == nextId - 1 {
      return true
    }
  }
  return false
}

func (gossiper *Gossiper) ForwardToAllPeers(sender *net.UDPAddr, packet *GossipPacket) {
  for _, peer := range gossiper.Peers {
    gossiper.SendPacket(peer, packet)
  }
}

func (gossiper *Gossiper) SendPacket(destination *net.UDPAddr, packet *GossipPacket) {
  packetBytes := EncodePacket(packet)

  gossiper.Conn.WriteToUDP(packetBytes, destination)
}

func (gossiper *Gossiper) UpdateRoute(sender *net.UDPAddr, msg *RumorMessage) {
  // Guard from routing yourself
  if msg.Origin != gossiper.Name {
    gossiper.Router[msg.Origin] = sender
    fmt.Println("DSDV", msg.Origin, sender.String())
  }
}

func (gossiper *Gossiper) MongerRumor(msg *RumorMessage, exclude *net.UDPAddr, isFlippedCoin bool) {
  if len(gossiper.Peers) == 0 {
    return;
  }
  // Forward message to random peer
  destination := gossiper.RandomPeer(exclude)
  gossiper.SendPacket(destination, &GossipPacket{nil, msg, nil, nil})
  if isFlippedCoin {
    fmt.Println("FLIPPED COIN sending rumor to", destination.String())
  }
  fmt.Println("MONGERING with", destination.String())
  gossiper.Timeouts[destination.String()] = utils.SetTimeout(func() {
    fmt.Println("TIMED OUT with", destination.String())
    gossiper.CoinFlip(msg, exclude)
  }, time.Second)
}

func (gossiper *Gossiper) SendRouteMessage() {
  rumor := &RumorMessage{
    gossiper.Name,
    gossiper.GetNextIDForOrigin(gossiper.Name),
    "",
  }
  gossiper.RecordRumor(rumor)
  gossiper.MongerRumor(rumor, nil, false)
}
func (gossiper *Gossiper) CoinFlip(msg *RumorMessage, exclude *net.UDPAddr) {
  // Pick a new random peer and start mongering
  if rand.Int() % 2 == 0 {
    gossiper.MongerRumor(msg, exclude, true)
  }
}
