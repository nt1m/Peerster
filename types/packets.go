package types

import (
  "fmt"
  "strconv"
  "github.com/dedis/protobuf"
  "github.com/nt1m/Peerster/utils"
  "encoding/json"
)

type SimpleMessage struct {
  OriginalName string
  RelayPeerAddr string
  Contents string
}

type RumorMessage struct {
  Origin string
  ID uint32
  Text string
}

type PrivateMessage struct {
  Origin string
  ID uint32
  Text string
  Destination string
  HopLimit uint32
}

type StatusPacket struct {
  Want []PeerStatus
}

type PeerStatus struct {
  Identifier string
  NextID uint32
}

type Message struct {
  Text string
  Destination string
}

type GossipPacket struct {
  Simple *SimpleMessage
  Rumor *RumorMessage
  Status *StatusPacket
  Private *PrivateMessage
}

func (packet* StatusPacket) ToMap() map[string]uint32 {
  statusMap := make(map[string]uint32)
  for _, status := range packet.Want {
    statusMap[status.Identifier] = status.NextID
  }
  return statusMap
}

func EncodePacket(packet *GossipPacket) []byte {
  packetBytes, err := protobuf.Encode(packet)
  utils.CheckError(err)
  return packetBytes
}

func (msg *SimpleMessage) Log() {
  fmt.Println("SIMPLE MESSAGE origin", msg.OriginalName, "from", msg.RelayPeerAddr, "contents", msg.Contents)
}

func (msg *RumorMessage) Log(relayAddress string) {
  fmt.Println("RUMOR origin", msg.Origin, "from", relayAddress, "ID", msg.ID, "contents", msg.Text)
}

func (msg *RumorMessage) ToJSON() string {
  bytes, err := json.Marshal(msg)
  utils.CheckError(err)
  return string(bytes)
}

func (msg *PrivateMessage) ToJSON() string {
  bytes, err := json.Marshal(msg)
  utils.CheckError(err)
  return string(bytes)
}

func (packet *GossipPacket) ToJSON() string {
  if packet.Rumor != nil {
    return packet.Rumor.ToJSON()
  }
  if packet.Private != nil {
    return packet.Private.ToJSON()
  }
  return "null"
}

func (packet *StatusPacket) Log(relayAddress string) {
  str := ""
  for i, status := range packet.Want {
    if i > 0 {
      str += " "
    }
    str += "peer " + status.Identifier + " nextID " + strconv.FormatUint(uint64(status.NextID), 10)
  }
  fmt.Println("STATUS from", relayAddress, str)
}

func (packet *PrivateMessage) Log() {
  fmt.Println("PRIVATE origin", packet.Origin, "hop-limit", packet.HopLimit, "contents", packet.Text)
}
