package main

import (
	"bytes"
	"fmt"
	"log"
	"os"
	"strconv"
	"strings"
)

const (
	DATAPATH_PREFIX = "idx" // Input prefix.
	PORT_PREFIX     = "p"
)

type Connection struct {
	port     int
	datapath int
}

func (c Connection) String() string {
	return fmt.Sprintf("{port: %d, datapath: %d}", c.port, c.datapath)
}

// Port returns a string for a port role.
func Port(num int) string { return fmt.Sprintf("%s%d", PORT_PREFIX, num) }

// Datapath returns a string for a datapath role.
func Datapath(num int) string { return fmt.Sprintf("%s%d", DATAPATH_PREFIX, num) }

func main() {
	if len(os.Args) < 2 {
		log.Fatalf("Not enough arguments (want=2, got=%d)\n", len(os.Args))
	}

	numDataPath, err := strconv.Atoi(os.Args[1])
	if err != nil {
		log.Fatal(err.Error())
	}

	fmt.Printf("// Input: %d datapaths\n", numDataPath)

	scribble, err := os.OpenFile("sched.scr", os.O_WRONLY|os.O_CREATE, 0600)
	if err != nil {
		panic(err)
	}
	defer func() {
		scribble.Close()
	}()

	cFile, err := os.OpenFile("sched.c", os.O_WRONLY|os.O_CREATE, 0600)
	if err != nil {
		panic(err)
	}
	defer func() {
		cFile.Close()
	}()

	code, protocol := genSched(numDataPath)
	scribble.WriteString(protocol)
	cFile.WriteString(code)
}

// notMsg is an expression to calculate (datapath % N == port) optimised for H/W
func notMsg(datapath, port, N int) string {
	if N > 0 && (port&(port-1) == 0) { // Power of 2
		return fmt.Sprintf("((%s&%d)^%d)", Datapath(datapath), N-1, port)
	} else {
		return fmt.Sprintf("((%s%%%d)^%d)", Datapath(datapath), N, port)
	}
}

// genSched generates a scheduler file and a protocol.
func genSched(N int) (string, string) {
	sched := new(bytes.Buffer)
	scrib := new(bytes.Buffer)

	baseConn := make([][]Connection, N)
	baseCond := make([][]string, N) // Base connection pairs (negated).
	msgPris := make([][]int, N)     // Priorities of each pair.

	for port := 0; port < N; port++ {
		baseCond[port] = make([]string, N)
		baseConn[port] = make([]Connection, N)
		msgPris[port] = make([]int, N)
		for priority := 0; priority < N; priority++ {
			datapath := (port + priority) % N
			baseCond[port][priority] = notMsg(datapath, port, N)
			baseConn[port][priority] = Connection{port: port, datapath: datapath}

			// Assign predefined priority to each port-datapath pair.
			msgPris[port][datapath] = priority
		}
	}

	protocol := make([]string, N) // Protocol per port.
	cond := make([][]string, N)
	for port := 0; port < N; port++ {
		cond[port] = make([]string, N)
		for datapath := 0; datapath < N; datapath++ {
			cond[port][datapath] = fmt.Sprintf("!(%s)", baseCond[port][msgPris[port][datapath]])
			for priority := msgPris[port][datapath] - 1; priority >= 0; priority-- {
				cond[port][datapath] += fmt.Sprintf(" && %s", baseCond[port][priority])
			}
		}
		// choice block nesting based on priority
		for priority := 0; priority < N; priority++ {
			datapath := (port + priority) % N
			protocol[port] += fmt.Sprintf("choice at %s {\n", Datapath(datapath))
			protocol[port] += fmt.Sprintf("  use%d_%d() from %s to %s;\n", datapath, port, Datapath(baseConn[port][msgPris[port][datapath]].datapath), Port(baseConn[port][msgPris[port][datapath]].port))
			for i := priority; i < N-1; i++ {
				protocol[port] += fmt.Sprintf("  T_%d_%d() from %s to %s; // Propagate\n", datapath, port, Datapath(datapath), Datapath((port+(i+1))%N))
			}
			protocol[port] += fmt.Sprintf("} or {\n")
			protocol[port] += fmt.Sprintf("  off%d_%d() from %s to %s;\n", datapath, port, Datapath(baseConn[port][msgPris[port][datapath]].datapath), Port(baseConn[port][msgPris[port][datapath]].port))
			for i := priority; i < N-1; i++ {
				protocol[port] += fmt.Sprintf("  F_%d_%d() from %s to %s; // Propagate\n", datapath, port, Datapath(datapath), Datapath((port+(i+1))%N))
			}
		}
		protocol[port] += fmt.Sprintf("%s\n", strings.Repeat("}", N))
	}

	// Scheduler header.
	sched.WriteString(fmt.Sprintf("void sched%d(", N))
	for i := 0; i < N; i++ {
		sched.WriteString(fmt.Sprintf("unsigned int %s, ", Datapath(i)))
	}
	sched.WriteString(fmt.Sprintf("int *enabled)\n{\n"))

	// Protocol header.
	scrib.WriteString(fmt.Sprintf("module Sched;\nglobal protocol Sched%d(", N))
	for i := 0; i < N; i++ {
		if i > 0 {
			scrib.WriteString(", ")
		}
		scrib.WriteString(fmt.Sprintf("role %s, role %s", Port(i), Datapath(i)))
	}
	scrib.WriteString(") {\n") // Protocol block.

	for port := 0; port < N; port++ {
		sched.WriteString(fmt.Sprintf("  enabled[%d] = ", port))
		for datapath := 0; datapath < N; datapath++ {
			if datapath != 0 {
				sched.WriteString("\n            || ")
			}
			sched.WriteString(fmt.Sprintf("(%s)", cond[port][datapath]))
		}
		sched.WriteString(";\n")

		scrib.WriteString(fmt.Sprintf("// Path %d\n%s\n", port, protocol[port]))
	}

	sched.WriteString("}\n")
	scrib.WriteString("}\n")
	return sched.String(), scrib.String()
}
