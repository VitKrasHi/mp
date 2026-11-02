package net

import (
	"bytes"
	"encoding/binary"
	"encoding/gob"
	"fmt"
	"io"
)

const DefaultTCPPort = 25000

type MsgType uint8

const (
	MsgJoin MsgType = iota
	MsgWelcome
	MsgInput
	MsgState
)

type Message struct {
	Type    MsgType
	Join    *JoinMsg
	Welcome *WelcomeMsg
	Input   *InputMsg
	State   *StateMsg
}

type JoinMsg struct{ Name string }

type WelcomeMsg struct {
	PlayerID, MaxPlayers int
	PlayerName, GameName string
}

type InputMsg struct{ DX, DY float64 }

type StateMsg struct{ Players []PlayerState }

type PlayerState struct {
	ID   uint32
	Name string
	X, Y float64
}

const maxMsgSize = 1 << 20

func RegisterGobTypes() {
	gob.Register(&Message{})
	gob.Register(&JoinMsg{})
	gob.Register(&WelcomeMsg{})
	gob.Register(&InputMsg{})
	gob.Register(&StateMsg{})
}

func WriteMessage(w io.Writer, m *Message) error {
	var buf bytes.Buffer
	if err := gob.NewEncoder(&buf).Encode(m); err != nil {
		return err
	}
	if buf.Len() > maxMsgSize {
		return fmt.Errorf("message too large")
	}
	var lb [4]byte
	binary.LittleEndian.PutUint32(lb[:], uint32(buf.Len()))
	if _, err := w.Write(lb[:]); err != nil {
		return err
	}
	_, err := w.Write(buf.Bytes())
	return err
}

func ReadMessage(r io.Reader) (*Message, error) {
	var lb [4]byte
	if _, err := io.ReadFull(r, lb[:]); err != nil {
		return nil, err
	}
	size := binary.LittleEndian.Uint32(lb[:])
	if size > maxMsgSize {
		return nil, fmt.Errorf("message too large")
	}
	buf := make([]byte, size)
	if _, err := io.ReadFull(r, buf); err != nil {
		return nil, err
	}
	var m Message
	if err := gob.NewDecoder(bytes.NewReader(buf)).Decode(&m); err != nil {
		return nil, err
	}
	return &m, nil
}
