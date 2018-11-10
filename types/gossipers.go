package types

import (
  "os"
  "net"
  "fmt"
  "time"
  "strings"
  "math/rand"
  "encoding/hex"
  "crypto/sha256"
  "github.com/nt1m/Peerster/utils"
)

var FILE_CHUNK_SIZE = int64(8192)

type File struct {
  FileName string
  FileSize int64
  NumChunks int64
  Chunks map[string][]byte // Map[Hash -> Bytes]
  MetaHash []byte
  MetaFile []byte
  Status int64
}

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
  DataRequestTimeouts map[string](chan bool)
  Files map[string]*File // Map[Hash -> File]
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
    Files: make(map[string]*File),
    Timeouts: make(map[string](chan bool)),
    DataRequestTimeouts: make(map[string](chan bool)),
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
    gossiper.VisibleMessages = append(gossiper.VisibleMessages, &GossipPacket{nil, rm, nil, nil, nil, nil})
  }
}

func (gossiper* Gossiper) RecordPrivate(msg *PrivateMessage) {
  gossiper.VisibleMessages = append(gossiper.VisibleMessages, &GossipPacket{nil, nil, nil, msg, nil, nil})
}

func (gossiper* Gossiper) ForwardPrivate(pm *PrivateMessage) {
  if pm.HopLimit > 0 {
    gossiper.SendPacket(
      gossiper.Router[pm.Destination],
      &GossipPacket{nil, nil, nil, pm, nil, nil})
  }
}

func (gossiper* Gossiper) ReplyDataRequest(rq *DataRequest) {
  key := hex.EncodeToString(rq.HashValue)
  if gossiper.Files[key] != nil {
    file := gossiper.Files[key]

    fmt.Println("ReplyDataRequest: ", key, "found")
    gossiper.SendPacket(
      gossiper.Router[rq.Origin],
      &GossipPacket{nil, nil, nil, nil, nil, &DataReply{
        Origin: gossiper.Name,
        Destination: rq.Origin,
        HashValue: rq.HashValue,
        Data: file.MetaFile,
      }},
    )
  } else {
    // Lookup file chunk with right hash.
    for _, file := range gossiper.Files {
      if file.Chunks[key] != nil {
        gossiper.SendPacket(
          gossiper.Router[rq.Origin],
          &GossipPacket{nil, nil, nil, nil, nil, &DataReply{
            Origin: gossiper.Name,
            Destination: rq.Origin,
            HashValue: rq.HashValue,
            Data: file.Chunks[key],
          }},
        )
      } else {
        fmt.Println("ReplyDataRequest: FAILED TO FIND CHUNK WITH HASH", hex.EncodeToString(rq.HashValue))
      }
    }
  }
}

func (gossiper* Gossiper) ForwardDataRequest(rq *DataRequest) {
  if rq.HopLimit > 0 {
    gossiper.SendPacket(
      gossiper.Router[rq.Destination],
      &GossipPacket{nil, nil, nil, nil, rq, nil})
  }
}

func (gossiper* Gossiper) SendDataRequest(rq *DataRequest) {
  gossiper.SendPacket(
    gossiper.Router[rq.Destination],
    &GossipPacket{nil, nil, nil, nil, rq, nil})
  gossiper.DataRequestTimeouts[hex.EncodeToString(rq.HashValue)] = utils.SetTimeout(func() {
    gossiper.SendDataRequest(rq)
  }, time.Second * 5)
}

func (gossiper* Gossiper) ProcessDataReply(rp *DataReply) {
  dataChecksum := sha256.Sum256(rp.Data)
  dataChecksumStr := hex.EncodeToString(dataChecksum[:])
  key := hex.EncodeToString(rp.HashValue)
  if dataChecksumStr != key {
    fmt.Println("WRONG CHECKSUM")
    return
  }
  if gossiper.DataRequestTimeouts[key] != nil {
    close(gossiper.DataRequestTimeouts[key])
    gossiper.DataRequestTimeouts[key] = nil
  }
  if gossiper.Files[key] != nil && gossiper.Files[key].Status == -1 {
    var file *File = gossiper.Files[key]
    // Fill in metafile
    file.MetaFile = rp.Data
    file.NumChunks = int64(len(rp.Data)) / int64(32)
    fmt.Println("DOWNLOADING metafile of", file.FileName, "from", rp.Origin)

    // Fill in stub Chunks
    for offset := 0; offset < len(rp.Data); offset += 32 {
      hashSlice := file.MetaFile[offset:(offset + 32)]
      file.Chunks[hex.EncodeToString(hashSlice)] = []byte{}
    }
    file.Status = 0
    // Request first chunk
    gossiper.SendDataRequest(&DataRequest{
      Origin: gossiper.Name,
      Destination: rp.Origin,
      HopLimit: 10,
      HashValue: file.MetaFile[0:32],
    })
  } else {
    // Fill in unfilled file chunk
    var file *File = nil
    for _, f := range gossiper.Files {
      if f.Chunks[key] != nil && len(f.Chunks[key]) == 0 {
        file = f
      }
    }

    if file != nil {
      file.Chunks[key] = rp.Data
      file.Status++
      fmt.Println("DOWNLOADING", file.FileName, "chunk", file.Status, "from", rp.Origin)

      if (file.Status == file.NumChunks) {
        // Reconstruct the file locally when done downloading
        file.Reconstruct()
      } else {
        offset := file.Status * 32
        requested := file.MetaFile[offset:(offset + 32)]
        // Request next chunk
        gossiper.SendDataRequest(&DataRequest{
          Origin: gossiper.Name,
          Destination: rp.Origin,
          HopLimit: 10,
          HashValue: requested,
        })
      }
    } else {
      fmt.Println("DataReplyHandler: CAN'T FIND HASH", key)
    }
  }
}

func (gossiper* Gossiper) ForwardDataReply(rp *DataReply) {
  if rp.HopLimit > 0 {
    gossiper.SendPacket(
      gossiper.Router[rp.Destination],
      &GossipPacket{nil, nil, nil, nil, nil, rp})
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
    mapValue, exists := statusMap[origin]
    if mapValue == nextId - 1 {
      return gossiper.GetMessage(origin, nextId - 1)
    }
    firstMessage := gossiper.GetMessage(origin, 1)
    if !exists && firstMessage != nil {
      return firstMessage
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
  gossiper.SendPacket(destination, &GossipPacket{nil, msg, nil, nil, nil, nil})
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
  if rand.Int() % 2 == 0 && len(gossiper.Peers) > 1 {
    gossiper.MongerRumor(msg, exclude, true)
  }
}

func (gossiper* Gossiper) AddStubFile(hash string, decodedHash []byte, fileName string) {
  gossiper.Files[hash] = &File{
    FileName: fileName,
    FileSize: -1,
    MetaHash: decodedHash,
    MetaFile: nil,
    Chunks: make(map[string][]byte),
    Status: int64(-1),
  }
}

func (gossiper *Gossiper) AddFile(fileName string, fileSize int64, metaHash [32]byte, metaFile []byte, chunks map[string][]byte, status int64) {
  key := hex.EncodeToString(metaHash[:])
  gossiper.Files[key] = &File{
    FileName: fileName,
    FileSize: fileSize,
    MetaHash: metaHash[:],
    MetaFile: metaFile,
    NumChunks: int64(len(chunks)),
    Chunks: chunks,
    Status: status,
  }
  fmt.Println("UPLOADED file", key, "with", len(chunks), "chunks")
}

func (file *File) Reconstruct() {
  local, err := os.Create("_Downloads/" + file.FileName)
  utils.CheckError(err)
  defer local.Close()
  fileSize := 0
  for offset := 0; offset < len(file.MetaFile); offset += 32 {
    hashSlice := file.MetaFile[offset:(offset + 32)]
    n, err := local.Write(file.Chunks[hex.EncodeToString(hashSlice)]);
    utils.CheckError(err)
    fileSize += n
  }
  if file.FileSize != int64(fileSize) {
    file.FileSize = int64(fileSize)
  }
  fmt.Println("RECONSTRUCTED file", file.FileName)
}
