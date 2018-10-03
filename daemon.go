package main

import (
	"bytes"
	"encoding/binary"
	"fmt"
	// "log"
	"math/rand"
	"net"
	"os"
	"time"
)

const (
	Ping             = 0x01
	Ack              = 0x01 << 1
	MemInitRequest   = 0x01 << 2
	MemInitReply     = 0x01 << 3
	MemUpdateSuspect = 0x01 << 4
	MemUpdateResume  = 0x01 << 5
	MemUpdateLeave   = 0x01 << 6
	MemUpdateJoin    = 0x01 << 7
	StateAlive       = 0x01
	StateSuspect     = 0x01 << 1
	StateMonit       = 0x01 << 2
	StateIntro       = 0x01 << 3
	IntroducerIP     = "10.194.16.24"
	Port             = ":6666"
	DetectPeriod     = 500 * time.Millisecond
)

type Header struct {
	Type uint8
	Seq  uint16
	Zero uint8
}

var init_timer *time.Timer
var PingAckTimeout map[uint16]*time.Timer
var CurrentEntry *Member
var CurrentList *MemberList
var LocalIP string

// A trick to simply get local IP address
func getLocalIP() net.IP {
	dial, err := net.Dial("udp", "8.8.8.8:80")
	printError(err)
	localAddr := dial.LocalAddr().(*net.UDPAddr)
	dial.Close()

	return localAddr.IP
}

// Convert net.IP to uint32
func ip2int(ip net.IP) uint32 {
	return binary.BigEndian.Uint32(ip)
}
// Convert uint32 to net.IP 
func int2ip(binip uint32) net.IP {
	ip := make(net.IP, 4)
	binary.BigEndian.PutUint32(ip, binip)
	return ip
}

// Start the membership service and join in the group
func startService() bool {

	// Create a new log file or append to exit file
	// file, err := os.OpenFile(LocalIP+".log", os.O_RDWR|os.O_CREATE|os.O_APPEND, 0766)
	// if err != nil {
	// 	fmt.Println("[Error] Open or Create log file failed")
	// 	return false
	// }
	// logPrefix := "[" + LocalIP + "]: "
	// fmt = log.New(file, logPrefix, log.Ldate|log.Lmicroseconds|log.Lshortfile)

	LocalIP = getLocalIP().String()
	timestamp := time.Now().UnixNano()
	state := StateAlive
	CurrentEntry = &Member{uint64(timestamp), ip2int(getLocalIP()), uint8(state)}
	CurrentList = NewMemberList(10)
	CurrentList.Insert(CurrentEntry)

	if LocalIP == IntroducerIP {
		CurrentEntry.State |= (StateIntro | StateMonit)
	} else {
		// New member, send Init Request to the introducer
		initRequest(CurrentEntry)
	}

	return true
}

// UDP send
func udpSend(addr string, packet []byte) {
	conn, err := net.Dial("udp", addr)
	printError(err)
	defer conn.Close()

	conn.Write(packet)
}

// UDP Daemon loop task
func udpDaemon() {
	serverAddr := Port
	udpAddr, err := net.ResolveUDPAddr("udp4", serverAddr)
	printError(err)
	// Listen the request from client
	listen, err := net.ListenUDP("udp", udpAddr)
	printError(err)

	go func() {
		for {
			udpDaemonHandle(listen)
		}
	}()
}

func udpDaemonHandle(connect *net.UDPConn) {
	// Making a buffer to accept the grep command content from client
	buffer := make([]byte, 1024)
	n, addr, err := connect.ReadFromUDP(buffer)
	printError(err)

	// Seperate header and payload
	const HeaderLength = 4  // Header Length 4 bytes

	// Read header
	headerBinData := buffer[:HeaderLength]
	var header Header
	buf := bytes.NewReader(headerBinData)
	err = binary.Read(buf, binary.BigEndian, &header)
	printError(err)


	if header.Type&Ping != 0 {

		// Check whether this ping carries Init Request
		if header.Type&MemInitRequest != 0 {
			// Read payload
			payload := buffer[HeaderLength:n] 
			// Handle Init Request
			fmt.Printf("NEW MEMBER %s JOIN\n", addr.IP.String())
			initReply(addr.IP.String(), header.Seq, payload)

		} else if header.Type&MemUpdateSuspect != 0 {
			fmt.Printf("handle suspect\n")
		} else if header.Type&MemUpdateResume != 0 {
			fmt.Printf("handle resume\n")
		} else if header.Type&MemUpdateLeave != 0 {
			fmt.Printf("handle leave\n")
		} else if header.Type&MemUpdateJoin != 0 {
			fmt.Printf("handle join\n")
		} else {
			// Ping with no payload, 
			// No handling payload needed
			// Check whether update sending needed
			// If no, simply reply with ack
			ack(addr.IP.String(), header.Seq)
		}


	} else if header.Type&Ack != 0 {

		// Receive Ack, stop ping timer
		stop := PingAckTimeout[header.Seq-1].Stop()
		if stop {
			fmt.Printf("RECEIVE ACK [%s]: %d\n", addr.IP.String(), header.Seq)
			delete(PingAckTimeout, header.Seq-1)	
		}

		if header.Type&MemInitReply != 0 {
			// Ack carries Init Reply, stop init timer
			stop := init_timer.Stop()
			if stop {
				fmt.Printf("RECEIVE INIT REPLY FROM [%s]: %d\n", addr.IP.String(), header.Seq)
			}
			// Read payload
			payload := buffer[HeaderLength:n] 
			// Retrive data from Init Reply and store them into the memberlist
			handleInitReply(payload)

		} else if header.Type&MemUpdateSuspect != 0 {
			fmt.Printf("handle suspect\n")
		} else if header.Type&MemUpdateResume != 0 {
			fmt.Printf("handle resume\n")
		} else if header.Type&MemUpdateLeave != 0 {
			fmt.Printf("handle leave\n")
		} else if header.Type&MemUpdateJoin != 0 {
			fmt.Printf("handle join\n")
		} else {
			fmt.Printf("receive pure ack\n")
		} 
	}
}

func handleInitReply(payload []byte) {
	//num := len(payload)
	fmt.Printf("len of payload: %d", len(payload))
}

func initReply(addr string, seq uint16, payload []byte) {
	// Read and insert new member to the memberlist
	var member *Member
	buf := bytes.NewReader(payload)
	err := binary.Read(buf, binary.BigEndian, member)
	printError(err)
	// Update state of the new member
	// ,,,
	CurrentList.Insert(member)

	// Put the entire memberlist to the Init Reply's payload
	var memBuffer bytes.Buffer  // Temp buf to store member's binary value
	var binBuffer bytes.Buffer
	// DEBUG
	fmt.Printf("list size: %d\n", CurrentList.Size())

	for i := 0; i < CurrentList.Size(); i += 1 {
		member = CurrentList.RetrieveByIdx(i)
		// DEBUG
		fmt.Printf("ts of %d: %d\n", i, member.TimeStamp)

		binary.Write(&memBuffer, binary.BigEndian, member)
		binBuffer.Write(memBuffer.Bytes())
		memBuffer.Reset()  // Clear buffer
	}

	// DEBUG PRINTLIST
	fmt.Printf("len of payload before send reply: %d\n", len(binBuffer.Bytes()))
	CurrentList.PrintMemberList()

	// Send pigggback Init Reply
	ackWithPayload(addr, seq, binBuffer.Bytes(), MemInitReply)
}

func initRequest(member *Member) {
	// Construct Init Request payload
	var binBuffer bytes.Buffer
	binary.Write(&binBuffer, binary.BigEndian, member)

	// Send piggyback Init Request
	pingWithPayload(IntroducerIP + Port, binBuffer.Bytes(), MemInitRequest)

	// Start Init timer, if expires, exit process
	init_timer = time.NewTimer(2 * time.Second)
	go func() {
		<-init_timer.C
		fmt.Printf("INIT %s TIMEOUT, PROCESS EXIT.\n", IntroducerIP)
		os.Exit(1)
	}()
}

func ackWithPayload(addr string, seq uint16, payload []byte, flag uint8) {
	packet := Header{Ack | flag, seq + 1, 0}
	var binBuffer bytes.Buffer
	binary.Write(&binBuffer, binary.BigEndian, packet)

	if payload != nil {
		binBuffer.Write(payload) // Append payload
		udpSend(addr+Port, binBuffer.Bytes())
	} else {
		udpSend(addr+Port, binBuffer.Bytes())
	}
}

func ack(addr string, seq uint16) {
	ackWithPayload(addr, seq, nil, 0x00)
}

func pingWithPayload(addr string, payload []byte, flag uint8) {
	// Source for genearting random number
	randSource := rand.NewSource(time.Now().UnixNano())
	randGen := rand.New(randSource)
	seq := randGen.Intn(0x01<<15 - 2)

	packet := Header{Ping | flag, uint16(seq), 0}
	var binBuffer bytes.Buffer
	binary.Write(&binBuffer, binary.BigEndian, packet)

	if payload != nil {
		binBuffer.Write(payload) // Append payload
		udpSend(addr, binBuffer.Bytes())
	} else {
		udpSend(addr, binBuffer.Bytes())
	}
	fmt.Printf("PING [%s]: %d\n", addr, seq)

	timer := time.NewTimer(time.Second)
	PingAckTimeout[uint16(seq)] = timer
	go func() {
		<-PingAckTimeout[uint16(seq)].C
		fmt.Printf("PING [%s]: %d TIMEOUT\n", addr, seq)
		delete(PingAckTimeout, uint16(seq))
	}()
}

func ping(addr string) {
	pingWithPayload(addr, nil, 0x00)
}

// Main func
func main() {
	PingAckTimeout = make(map[uint16]*time.Timer)
	if startService() == true {
		fmt.Printf("START SERVICE\n")
	}
	udpDaemon()
	for {
		ping("10.194.16.24" + Port)
		time.Sleep(DetectPeriod)
	}
}

// Helper function to print the err in process
func printError(err error) {
	if err != nil {
		fmt.Println("[ERROR]", err.Error())
	}
}
